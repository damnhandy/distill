package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/builder"
)

const releasesURL = "https://github.com/damnhandy/distill#dependencies"

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that required runtime tools are installed",
		Long: `doctor inspects your PATH for the external tools that distill shells out to
and reports which are present, missing, or outdated.

For each missing or outdated tool it prints an install or upgrade command
appropriate for the package manager detected on your system (Devbox, Nix,
Homebrew, dnf, apt-get, or apk). When no package is available through a
package manager, a link to the project's GitHub releases page is shown instead.

Tools checked:
  podman / docker   required by: build          (min: podman 4.0.0 / docker 20.10.0)
  cosign            required by: provenance      (min: 2.0.0)
  syft              required by: attest          (min: 1.0.0)
  grype             required by: scan            (min: 0.70.0)
  skopeo            optional  — enriches provenance with base-image digests (min: 1.0.0)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd)
		},
	}
}

func runDoctor(cmd *cobra.Command) error {
	ctx := cmd.Context()
	pm := builder.DetectedPackageManager()

	// tabwriter buffers all writes internally; only Flush can return a non-nil
	// error, so the individual write return values are intentionally discarded.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tSTATUS\tVERSION\tPATH / NOTE")
	_, _ = fmt.Fprintln(tw, "────\t──────\t───────\t───────────")

	allOK := true

	// Container runtime row — special-cased because it is an either/or choice.
	cli, cliPath, cliVer, cliVerOK, runtimeFound := builder.ContainerRuntimeStatus(ctx)
	switch {
	case !runtimeFound:
		allOK = false
		_, _ = fmt.Fprintf(tw, "%s\t✗ missing\t\t(required by: build)\n", cli)
	case !cliVerOK:
		allOK = false
		_, _ = fmt.Fprintf(tw, "%s\t✗ outdated\t%s\t%s\n", cli, cliVer, cliPath)
	default:
		_, _ = fmt.Fprintf(tw, "%s\t✓ ok\t%s\t%s\n", cli, cliVer, cliPath)
	}

	// All other tools — skip the individual podman/docker rows, covered above.
	statuses := builder.CheckAll(ctx)
	for _, s := range statuses {
		if s.Name == "podman" || s.Name == "docker" {
			continue
		}
		switch {
		case !s.Found:
			allOK = false
			tag := "required"
			if !s.Required {
				tag = "optional"
			}
			_, _ = fmt.Fprintf(tw, "%s\t✗ missing\t\t(%s — used by: %s)\n",
				s.Name, tag, strings.Join(s.UsedBy, ", "))
		case !s.VersionOK:
			allOK = false
			_, _ = fmt.Fprintf(tw, "%s\t✗ outdated\t%s\t%s\n", s.Name, s.Version, s.Path)
		default:
			_, _ = fmt.Fprintf(tw, "%s\t✓ ok\t%s\t%s\n", s.Name, s.Version, s.Path)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	if allOK {
		fmt.Println("\nAll tools found and up to date. distill is ready to use.")
		return nil
	}

	// Print install/upgrade hints for problem tools.
	fmt.Println()
	if pm != "" {
		fmt.Printf("Detected package manager: %s\n\n", pm)
	}

	// Missing container runtime.
	if !runtimeFound {
		fmt.Println("Install missing tools:")
		printHint(string(cli), pm)
	} else if !cliVerOK {
		fmt.Printf("Upgrade outdated tools (installed: %s, required ≥ %s):\n",
			cliVer, builder.MinVersionFor(string(cli)))
		printHint(string(cli), pm)
	}

	hasMissing, hasOutdated := false, false
	for _, s := range statuses {
		if s.Name == "podman" || s.Name == "docker" {
			continue
		}
		if !s.Found {
			hasMissing = true
		} else if !s.VersionOK {
			hasOutdated = true
		}
	}

	if hasMissing {
		fmt.Println("Install missing tools:")
		for _, s := range statuses {
			if s.Name == "podman" || s.Name == "docker" || s.Found {
				continue
			}
			printHint(s.Name, pm)
		}
	}
	if hasOutdated {
		fmt.Println("Upgrade outdated tools:")
		for _, s := range statuses {
			if s.Name == "podman" || s.Name == "docker" || !s.Found || s.VersionOK {
				continue
			}
			fmt.Printf("  # installed: %s, required ≥ %s\n", s.Version, s.MinVersion)
			printHint(s.Name, pm)
		}
	}

	return nil
}

func printHint(tool, pm string) {
	hint := builder.InstallHint(tool, pm)
	if hint != "" {
		fmt.Printf("  %s\n", hint)
	} else {
		fmt.Printf("  %-10s  →  %s\n", tool, releasesURL)
	}
	if tool == "docker" {
		printDockerDesktopHint()
	}
}

// printDockerDesktopHint prints a note mapping the Docker CLI minimum version
// to the Docker Desktop release that first shipped it. Only shown on platforms
// where Docker Desktop is the normal installation method.
func printDockerDesktopHint() {
	switch runtime.GOOS {
	case "darwin", "windows":
		fmt.Printf("  # Docker CLI %s+ is provided by Docker Desktop 3.0.0 or later\n",
			builder.MinVersionFor("docker"))
		fmt.Println("  # https://www.docker.com/products/docker-desktop/")
	}
}
