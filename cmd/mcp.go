package cmd

import (
	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/mcpserver"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP stdio server for agent integration",
		Long: `mcp starts a Model Context Protocol (MCP) stdio server that exposes
distill's capabilities as typed tools. Any MCP-compatible agent or client
(Claude Code, Claude Desktop, Cursor, custom agents) can connect to it by
running this command as a subprocess.

Tools exposed:
  validate_spec      — parse and validate a .distill.yaml YAML string
  scaffold_spec      — generate a spec scaffold for a base distribution
  build_image        — build a minimal OCI image from a spec file on disk
  publish_image      — build → scan → push → SBOM → provenance
  scan_image         — scan an image for CVEs (structured JSON findings)
  generate_sbom      — generate an SPDX SBOM for an image
  check_dependencies — check which runtime tools are installed

Resources:
  distill://spec/schema  — JSON Schema for .distill.yaml ImageSpec
  distill://bases        — supported base distributions and their defaults

To use with Claude Code, add to .mcp.json in your project:
  {
    "mcpServers": {
      "distill": {
        "command": "distill",
        "args": ["mcp"]
      }
    }
  }`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			s := mcpserver.New(Version)
			return s.Serve()
		},
	}
}
