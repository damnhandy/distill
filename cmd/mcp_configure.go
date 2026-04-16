package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newMCPConfigureCmd() *cobra.Command {
	var (
		tool  string
		scope string
	)

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Register the distill MCP server with an AI coding agent",
		Long: `configure writes the configuration file needed to register
the distill MCP server with your AI coding agent.

Supported agents:
  claude   — Claude Code
               project scope: .mcp.json  (commit to share with your team)
               user scope:    ~/.claude.json  (all projects on this machine)

  copilot  — GitHub Copilot in VS Code
               project scope: .vscode/mcp.json  (commit to share with your team)
               user scope:    VS Code user config directory (all workspaces)

Running with --scope project (default) writes to the current directory.
Running with --scope user writes to your home / user profile directory.`,
		Example: `  # Register for Claude Code in the current project (recommended)
  distill mcp configure --tool claude

  # Register globally for Claude Code (all projects)
  distill mcp configure --tool claude --scope user

  # Register for GitHub Copilot in VS Code in the current project
  distill mcp configure --tool copilot

  # Register globally for GitHub Copilot in VS Code
  distill mcp configure --tool copilot --scope user`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMCPConfigure(tool, scope)
		},
	}

	cmd.Flags().StringVarP(&tool, "tool", "t", "",
		`Agent to configure: "claude" (Claude Code) or "copilot" (GitHub Copilot in VS Code)`)
	cmd.Flags().StringVarP(&scope, "scope", "s", "project",
		`Configuration scope: "project" (current directory, default) or "user" (global)`)
	_ = cmd.MarkFlagRequired("tool")

	return cmd
}

func runMCPConfigure(tool, scope string) error {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "claude":
		return configureClaude(scope)
	case "copilot":
		return configureCopilot(scope)
	default:
		return fmt.Errorf("unknown tool %q — supported values: claude, copilot", tool)
	}
}

// configureClaude writes (or merges into) the Claude Code MCP config file.
//
//	project scope → .mcp.json in the current directory
//	user scope    → ~/.claude.json
func configureClaude(scope string) error {
	var path string
	switch scope {
	case "project":
		path = ".mcp.json"
	case "user":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolving home directory: %w", err)
		}
		path = filepath.Join(home, ".claude.json")
	default:
		return fmt.Errorf("unknown scope %q — supported values: project, user", scope)
	}

	entry := map[string]any{
		"command": "distill",
		"args":    []string{"mcp"},
	}

	existed, updated, err := mergeServerEntry(path, "mcpServers", "distill", entry)
	if err != nil {
		return err
	}

	printConfigResult("Claude Code", scope, path, existed, updated)
	if scope == "project" {
		fmt.Println("  Commit .mcp.json to share this configuration with your team.")
	}
	return nil
}

// configureCopilot writes (or merges into) the GitHub Copilot VS Code MCP config file.
//
//	project scope → .vscode/mcp.json in the current directory
//	user scope    → VS Code user config directory (platform-dependent)
func configureCopilot(scope string) error {
	var path string
	switch scope {
	case "project":
		if err := os.MkdirAll(".vscode", 0o750); err != nil {
			return fmt.Errorf("creating .vscode directory: %w", err)
		}
		path = filepath.Join(".vscode", "mcp.json")
	case "user":
		var err error
		path, err = vsCodeUserMCPPath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("creating VS Code config directory: %w", err)
		}
	default:
		return fmt.Errorf("unknown scope %q — supported values: project, user", scope)
	}

	// VS Code / Copilot uses "servers" (not "mcpServers") and requires type: "stdio".
	entry := map[string]any{
		"type":    "stdio",
		"command": "distill",
		"args":    []string{"mcp"},
	}

	existed, updated, err := mergeServerEntry(path, "servers", "distill", entry)
	if err != nil {
		return err
	}

	printConfigResult("GitHub Copilot", scope, path, existed, updated)
	fmt.Println("  Reload VS Code (Cmd/Ctrl+Shift+P → \"Developer: Reload Window\") to activate.")
	if scope == "project" {
		fmt.Println("  Commit .vscode/mcp.json to share this configuration with your team.")
	}
	return nil
}

// mergeServerEntry reads the JSON file at path (if it exists), sets
// root[topKey][serverName] = entry, and writes the result back as indented JSON.
//
// Returns:
//   - existed: true if the file already existed before this call
//   - updated: true if the file was written (false when the entry was already identical)
func mergeServerEntry(path, topKey, serverName string, entry map[string]any) (existed, updated bool, err error) {
	raw := map[string]any{}

	data, readErr := os.ReadFile(path) //nolint:gosec // path is always a hardcoded config location
	if readErr == nil {
		existed = true
		if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
			return existed, false, fmt.Errorf("parsing %s: %w", path, jsonErr)
		}
	}

	servers, _ := raw[topKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}

	// Skip write when the entry is already present and identical.
	if existing, ok := servers[serverName]; ok {
		existingJSON, _ := json.Marshal(existing)
		newJSON, _ := json.Marshal(entry)
		if string(existingJSON) == string(newJSON) {
			return existed, false, nil
		}
	}

	servers[serverName] = entry
	raw[topKey] = servers

	out, marshalErr := json.MarshalIndent(raw, "", "  ")
	if marshalErr != nil {
		return existed, false, fmt.Errorf("encoding JSON: %w", marshalErr)
	}
	if writeErr := os.WriteFile(path, append(out, '\n'), 0o600); writeErr != nil {
		return existed, false, fmt.Errorf("writing %s: %w", path, writeErr)
	}
	return existed, true, nil
}

// printConfigResult prints a summary line after a configure operation.
func printConfigResult(agent, scope, path string, existed, updated bool) {
	display := abbreviateHome(path)
	switch {
	case !existed:
		fmt.Printf("Configured distill MCP server for %s (%s scope)\n", agent, scope)
		fmt.Printf("  File: %s\n", display)
	case !updated:
		fmt.Printf("distill MCP server already configured for %s (%s scope) — no changes made\n", agent, scope)
		fmt.Printf("  File: %s\n", display)
	default:
		fmt.Printf("Updated distill MCP server configuration for %s (%s scope)\n", agent, scope)
		fmt.Printf("  File: %s\n", display)
	}
}

// abbreviateHome replaces the home directory prefix in path with "~".
func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return "~" + string(filepath.Separator) + rel
}

// vsCodeUserMCPPath returns the platform-specific path for the VS Code user-level
// MCP configuration file (~/.config/Code/User/mcp.json on Linux, etc.).
func vsCodeUserMCPPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "Code", "User")
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		dir = filepath.Join(appdata, "Code", "User")
	default: // linux and others
		dir = filepath.Join(home, ".config", "Code", "User")
	}
	return filepath.Join(dir, "mcp.json"), nil
}
