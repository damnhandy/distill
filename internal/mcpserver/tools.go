package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/damnhandy/distill/internal/builder"
	"github.com/damnhandy/distill/internal/spec"
)

// withStdoutToStderr temporarily redirects os.Stdout to os.Stderr for the
// duration of fn. The MCP stdio transport uses os.Stdout for JSON-RPC framing;
// any writes to os.Stdout during a tool call would corrupt the protocol stream.
// The builder functions write progress to os.Stdout, so we redirect them.
func withStdoutToStderr(fn func() error) error {
	orig := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = orig }()
	return fn()
}

// toolError wraps an error as an MCP tool error result.
func toolError(format string, args ...any) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...))
}

// handleValidateSpec parses and validates a spec YAML string.
func handleValidateSpec(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	specYAML, _ := args["spec_yaml"].(string)
	if specYAML == "" {
		return toolError("spec_yaml is required"), nil
	}

	type result struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors,omitempty"`
	}

	_, err := spec.Parse([]byte(specYAML))
	if err != nil {
		return mcp.NewToolResultJSON(result{
			Valid:  false,
			Errors: []string{err.Error()},
		})
	}
	return mcp.NewToolResultJSON(result{Valid: true})
}

// handleScaffoldSpec generates a .distill.yaml YAML string.
func handleScaffoldSpec(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name, _ := args["name"].(string)
	base, _ := args["base"].(string)
	variant, _ := args["variant"].(string)
	destination, _ := args["destination"].(string)

	if name == "" {
		return toolError("name is required"), nil
	}
	if base == "" {
		return toolError("base is required"), nil
	}
	if variant == "" {
		variant = "runtime"
	}

	content, err := spec.ScaffoldSpec(name, base, variant, destination)
	if err != nil {
		return toolError("%s", err), nil
	}

	type result struct {
		Content string `json:"content"`
	}
	return mcp.NewToolResultJSON(result{Content: content})
}

// handleBuildImage builds an OCI image from a spec file on disk.
func handleBuildImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	specPath, _ := args["spec_path"].(string)
	platform, _ := args["platform"].(string)

	if specPath == "" {
		return toolError("spec_path is required"), nil
	}

	data, err := os.ReadFile(specPath) //nolint:gosec // G304: spec_path is agent-provided
	if err != nil {
		return toolError("reading spec %q: %s", specPath, err), nil
	}

	imageSpec, err := spec.Parse(data)
	if err != nil {
		return toolError("invalid spec: %s", err), nil
	}

	if err := builder.CheckDeps(); err != nil {
		return toolError("dependency check failed: %s", err), nil
	}

	b, err := builder.New(imageSpec.Source.PackageManager)
	if err != nil {
		return toolError("%s", err), nil
	}

	platforms := imageSpec.EffectivePlatforms()
	if platform != "" {
		platforms = []string{platform}
	}

	buildErr := withStdoutToStderr(func() error {
		for _, p := range platforms {
			if err := b.Build(ctx, imageSpec, p); err != nil {
				return err
			}
		}
		return nil
	})
	if buildErr != nil {
		return toolError("build failed: %s", buildErr), nil
	}

	type result struct {
		ImageRef  string   `json:"image_ref"`
		Platforms []string `json:"platforms"`
	}
	ref := ""
	if imageSpec.Destination != nil {
		ref = imageSpec.Destination.Ref()
	}
	return mcp.NewToolResultJSON(result{ImageRef: ref, Platforms: platforms})
}

// handlePublishImage runs the full publish workflow.
func handlePublishImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	specPath, _ := args["spec_path"].(string)
	platform, _ := args["platform"].(string)
	skipBuild, _ := args["skip_build"].(bool)
	skipPipeline, _ := args["skip_pipeline"].(bool)

	if specPath == "" {
		return toolError("spec_path is required"), nil
	}

	data, err := os.ReadFile(specPath) //nolint:gosec // G304: spec_path is agent-provided
	if err != nil {
		return toolError("reading spec %q: %s", specPath, err), nil
	}

	imageSpec, err := spec.Parse(data)
	if err != nil {
		return toolError("invalid spec: %s", err), nil
	}

	if imageSpec.Destination == nil || imageSpec.Destination.Image == "" {
		return toolError("publish requires destination.image to be set in the spec"), nil
	}

	if err := builder.CheckDeps(); err != nil {
		return toolError("dependency check failed: %s", err), nil
	}

	platforms := imageSpec.EffectivePlatforms()
	if platform != "" {
		platforms = []string{platform}
	}
	image := imageSpec.Destination.Ref()

	publishErr := withStdoutToStderr(func() error {
		if !skipBuild {
			b, err := builder.New(imageSpec.Source.PackageManager)
			if err != nil {
				return err
			}
			for _, p := range platforms {
				if err := b.Build(ctx, imageSpec, p); err != nil {
					return fmt.Errorf("build: %w", err)
				}
			}
		}

		if !skipPipeline {
			if err := builder.RunPipeline(ctx, imageSpec, image, specPath, builder.PipelineModeLocal); err != nil {
				return err
			}
		}

		if err := builder.Push(ctx, image); err != nil {
			return err
		}

		if !skipPipeline {
			if err := builder.RunPipeline(ctx, imageSpec, image, specPath, builder.PipelineModePublish); err != nil {
				return err
			}
		}

		return nil
	})
	if publishErr != nil {
		return toolError("publish failed: %s", publishErr), nil
	}

	type result struct {
		ImageRef string `json:"image_ref"`
	}
	return mcp.NewToolResultJSON(result{ImageRef: image})
}

// handleScanImage scans an image for CVEs and returns structured findings.
func handleScanImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	imageRef, _ := args["image_ref"].(string)
	failOn, _ := args["fail_on"].(string)

	if imageRef == "" {
		return toolError("image_ref is required"), nil
	}
	if failOn == "" {
		failOn = "critical"
	}

	scanResult, err := builder.ScanJSON(ctx, imageRef, failOn)
	if err != nil {
		return toolError("scan failed: %s", err), nil
	}
	return mcp.NewToolResultJSON(scanResult)
}

// handleGenerateSBOM generates an SPDX SBOM for an image.
func handleGenerateSBOM(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	imageRef, _ := args["image_ref"].(string)
	outputPath, _ := args["output_path"].(string)

	if imageRef == "" {
		return toolError("image_ref is required"), nil
	}
	if outputPath == "" {
		outputPath = "sbom.spdx.json"
	}

	sbomErr := withStdoutToStderr(func() error {
		return builder.Attest(ctx, imageRef, outputPath)
	})
	if sbomErr != nil {
		return toolError("SBOM generation failed: %s", sbomErr), nil
	}

	type result struct {
		SBOMPath string `json:"sbom_path"`
	}
	return mcp.NewToolResultJSON(result{SBOMPath: outputPath})
}

// depStatus is the shape of a single dependency entry in the check_dependencies result.
type depStatus struct {
	Name      string   `json:"name"`
	Found     bool     `json:"found"`
	Path      string   `json:"path,omitempty"`
	Version   string   `json:"version,omitempty"`
	VersionOK bool     `json:"version_ok"`
	Required  bool     `json:"required"`
	UsedBy    []string `json:"used_by"`
}

// handleCheckDependencies reports the status of all distill runtime dependencies.
func handleCheckDependencies(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cli, cliPath, cliVer, cliVerOK, runtimeFound := builder.ContainerRuntimeStatus(ctx)
	all := builder.CheckAll(ctx)

	results := make([]depStatus, 0, len(all)+1)

	// Container runtime row first.
	results = append(results, depStatus{
		Name:      string(cli),
		Found:     runtimeFound,
		Path:      cliPath,
		Version:   cliVer,
		VersionOK: cliVerOK,
		Required:  true,
		UsedBy:    []string{"build", "publish"},
	})

	for _, s := range all {
		if s.Name == "podman" || s.Name == "docker" {
			continue // already covered by the runtime row
		}
		results = append(results, depStatus{
			Name:      s.Name,
			Found:     s.Found,
			Path:      s.Path,
			Version:   s.Version,
			VersionOK: s.VersionOK,
			Required:  s.Required,
			UsedBy:    s.UsedBy,
		})
	}

	raw, err := json.Marshal(results)
	if err != nil {
		return toolError("marshaling dependency status: %s", err), nil
	}
	return mcp.NewToolResultText(string(raw)), nil
}
