// Package mcpserver implements the distill MCP server. It exposes distill's
// build, publish, scan, and validation capabilities as typed MCP tools so that
// any MCP-compatible agent (Claude Code, Claude Desktop, custom agents) can
// drive distill programmatically without scraping CLI output.
package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server instance.
type Server struct {
	s *server.MCPServer
}

// New creates an MCP server with all distill tools and resources registered.
// version is embedded in the server info returned during MCP initialization.
func New(version string) *Server {
	s := server.NewMCPServer(
		"distill",
		version,
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
	)

	// ── Tools ────────────────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("validate_spec",
			mcp.WithDescription(
				"Parse and validate a distill .distill.yaml spec. Returns structured "+
					"validation errors so you can fix the spec before attempting a build. "+
					"Always call this before build_image or publish_image when generating a spec.",
			),
			mcp.WithString("spec_yaml",
				mcp.Required(),
				mcp.Description("Full YAML content of the .distill.yaml spec to validate"),
			),
		),
		handleValidateSpec,
	)

	s.AddTool(
		mcp.NewTool("scaffold_spec",
			mcp.WithDescription(
				"Generate a .distill.yaml spec scaffold for a given base distribution. "+
					"Returns the YAML content as a string; write it to a file with your "+
					"preferred tool, then call validate_spec to confirm it is valid.",
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Image name (e.g. myapp)"),
			),
			mcp.WithString("base",
				mcp.Required(),
				mcp.Description(
					"Base distribution shorthand (ubi9, ubi8, fedora, debian, ubuntu, ubuntu22) "+
						"or a fully qualified image URI such as rockylinux:9",
				),
			),
			mcp.WithString("variant",
				mcp.Description(`Image variant: "runtime" removes the package manager after `+
					`install (default); "dev" retains it`),
			),
			mcp.WithString("destination",
				mcp.Description(
					"Destination OCI image reference (e.g. myregistry.io/myapp:latest). "+
						"Sets destination.image and destination.releasever in the spec.",
				),
			),
		),
		handleScaffoldSpec,
	)

	s.AddTool(
		mcp.NewTool("build_image",
			mcp.WithDescription(
				"Build a minimal OCI image from a .distill.yaml spec file on disk. "+
					"Requires docker (macOS/Windows) or podman (Linux) to be installed. "+
					"Build output streams to stderr; returns the image reference on success.",
			),
			mcp.WithString("spec_path",
				mcp.Required(),
				mcp.Description("Path to the .distill.yaml spec file on disk"),
			),
			mcp.WithString("platform",
				mcp.Description(
					"Override build platform (e.g. linux/amd64). "+
						"Builds all platforms listed in the spec when omitted.",
				),
			),
		),
		handleBuildImage,
	)

	s.AddTool(
		mcp.NewTool("publish_image",
			mcp.WithDescription(
				"Run the full distill publish workflow: build → scan → push → SBOM → provenance. "+
					"Requires destination.image to be set in the spec and registry credentials "+
					"to be available to docker/podman.",
			),
			mcp.WithString("spec_path",
				mcp.Required(),
				mcp.Description("Path to the .distill.yaml spec file on disk"),
			),
			mcp.WithString("platform",
				mcp.Description("Override platform (e.g. linux/amd64)"),
			),
			mcp.WithBoolean("skip_build",
				mcp.Description("Skip the build step (assumes the image is already built locally)"),
			),
			mcp.WithBoolean("skip_pipeline",
				mcp.Description("Skip all pipeline steps; push the image only"),
			),
		),
		handlePublishImage,
	)

	s.AddTool(
		mcp.NewTool("scan_image",
			mcp.WithDescription(
				"Scan an OCI image for CVEs using Grype. Returns structured findings "+
					"with severity, package, version, and fix information. "+
					"passed is false when any finding meets or exceeds the fail_on threshold.",
			),
			mcp.WithString("image_ref",
				mcp.Required(),
				mcp.Description("OCI image reference to scan (e.g. myregistry.io/myapp:latest)"),
			),
			mcp.WithString("fail_on",
				mcp.Description(
					"Minimum severity that sets passed=false. "+
						"One of: critical (default), high, medium, low, negligible",
				),
			),
		),
		handleScanImage,
	)

	s.AddTool(
		mcp.NewTool("generate_sbom",
			mcp.WithDescription(
				"Generate an SPDX SBOM for an OCI image using Syft. "+
					"Writes the SBOM to output_path and returns that path.",
			),
			mcp.WithString("image_ref",
				mcp.Required(),
				mcp.Description("OCI image reference (e.g. myregistry.io/myapp:latest)"),
			),
			mcp.WithString("output_path",
				mcp.Description("Output file path for the SPDX JSON (default: sbom.spdx.json)"),
			),
		),
		handleGenerateSBOM,
	)

	s.AddTool(
		mcp.NewTool("check_dependencies",
			mcp.WithDescription(
				"Check which distill runtime tools are installed and whether their versions "+
					"meet the minimum requirements. Always run this first when setting up "+
					"distill in a new environment.",
			),
		),
		handleCheckDependencies,
	)

	// ── Resources ────────────────────────────────────────────────────────────

	s.AddResource(
		mcp.NewResource(
			"distill://spec/schema",
			"ImageSpec JSON Schema",
			mcp.WithMIMEType("application/schema+json"),
			mcp.WithResourceDescription(
				"JSON Schema for the distill .distill.yaml ImageSpec. "+
					"Use this to validate or generate specs correctly.",
			),
		),
		handleSpecSchema,
	)

	s.AddResource(
		mcp.NewResource(
			"distill://bases",
			"Supported base distributions",
			mcp.WithMIMEType("application/json"),
			mcp.WithResourceDescription(
				"List of supported base distribution shorthands, their source images, "+
					"package managers, and default values.",
			),
		),
		handleBases,
	)

	return &Server{s: s}
}

// Serve starts the MCP stdio server. It blocks until the client disconnects.
func (srv *Server) Serve() error {
	return server.ServeStdio(srv.s)
}
