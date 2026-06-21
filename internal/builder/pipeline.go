package builder

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// PipelineMode controls which steps RunPipeline will execute.
type PipelineMode int

const (
	// PipelineModeLocal runs scan and sbom — steps that operate on a locally
	// built image. Provenance is skipped because it requires the image to be
	// in a registry. Used by "distill build --pipeline".
	PipelineModeLocal PipelineMode = iota

	// PipelineModePublish runs sbom and provenance — the post-push steps.
	// Scan is skipped because it already ran pre-push in the publish workflow.
	// Used by "distill publish".
	PipelineModePublish
)

// RunPipeline executes the enabled pipeline steps declared in s.Pipeline.
//
// image is the OCI image reference to operate on. specPath is forwarded to
// Provenance for predicate enrichment. mode gates which steps run:
//
//   - PipelineModeLocal  — scan + sbom; provenance skipped (no push yet)
//   - PipelineModePublish — sbom + provenance; scan skipped (ran pre-push)
//
// Returns nil immediately when s.Pipeline is nil.
func RunPipeline(ctx context.Context, s *spec.ImageSpec, image, specPath string, mode PipelineMode) error {
	if s.Pipeline == nil {
		return nil
	}

	if mode == PipelineModeLocal {
		if sc := s.Pipeline.Scan; sc != nil && sc.Enabled {
			fmt.Printf("\n── Pipeline: scan ────────────────────────────────────────\n")
			fmt.Printf("Scanning %q (fail-on: %s)\n\n", image, sc.FailOn)
			if err := Scan(ctx, image, sc.FailOn); err != nil {
				return fmt.Errorf("pipeline scan: %w", err)
			}
		}
	}

	if sb := s.Pipeline.SBOM; sb != nil && sb.Enabled {
		fmt.Printf("\n── Pipeline: sbom ────────────────────────────────────────\n")
		fmt.Printf("Generating SBOM for %q -> %s\n\n", image, sb.Output)
		if err := Attest(ctx, image, sb.Output); err != nil {
			return fmt.Errorf("pipeline sbom: %w", err)
		}
	}

	if mode == PipelineModePublish {
		if pv := s.Pipeline.Provenance; pv != nil && pv.Enabled {
			fmt.Printf("\n── Pipeline: provenance ──────────────────────────────────\n")
			if err := Provenance(ctx, image, ProvenanceOptions{
				SpecPath:      specPath,
				PredicatePath: pv.Predicate,
			}); err != nil {
				return fmt.Errorf("pipeline provenance: %w", err)
			}
		}
	}

	return nil
}

// Push pushes a locally-built image to its registry using the detected
// container CLI (docker or podman). Output streams to stdout via run().
func Push(ctx context.Context, image string) error {
	cli := DetectCLI()
	fmt.Printf("Pushing %s ...\n\n", image)
	if err := run(ctx, os.Stdout, string(cli), "push", image); err != nil {
		return fmt.Errorf("push %s: %w", image, err)
	}
	return nil
}

// PlatformRef derives the per-platform image tag by appending a sanitized
// platform suffix to baseRef (e.g. "linux/amd64" → "image:tag-linux-amd64").
func PlatformRef(baseRef, platform string) string {
	return baseRef + "-" + strings.ReplaceAll(platform, "/", "-")
}

// PushManifest pushes each per-platform image to the registry, then
// assembles and pushes a multi-platform manifest index under baseRef.
// platRefs are derived from platforms via PlatformRef.
func PushManifest(ctx context.Context, baseRef string, platforms []string) error {
	cli := DetectCLI()

	platRefs := make([]string, len(platforms))
	for i, p := range platforms {
		platRefs[i] = PlatformRef(baseRef, p)
	}

	for _, ref := range platRefs {
		fmt.Printf("Pushing %s ...\n\n", ref)
		if err := run(ctx, os.Stdout, string(cli), "push", ref); err != nil {
			return fmt.Errorf("push %s: %w", ref, err)
		}
	}

	createArgs := append([]string{"manifest", "create", "--amend", baseRef}, platRefs...)
	if err := run(ctx, os.Stdout, string(cli), createArgs...); err != nil {
		return fmt.Errorf("manifest create %s: %w", baseRef, err)
	}

	if err := run(ctx, os.Stdout, string(cli), "manifest", "push", baseRef); err != nil {
		return fmt.Errorf("manifest push %s: %w", baseRef, err)
	}

	return nil
}
