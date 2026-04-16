package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/spec"
)

func newInitCmd() *cobra.Command {
	var (
		name        string
		base        string
		variant     string
		output      string
		destination string
		force       bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new .distill.yaml spec file",
		Long: `init generates a .distill.yaml scaffold with sensible defaults for
the chosen base distribution.

--base accepts either a shorthand key or a fully qualified image URI:

  Shorthand keys:
    ubi9      — Red Hat UBI 9 (DNF)
    ubi8      — Red Hat UBI 8 (DNF)
    fedora    — Fedora latest (DNF)
    debian    — Debian Bookworm slim (APT)
    ubuntu    — Ubuntu 24.04 LTS (APT)
    ubuntu22  — Ubuntu 22.04 LTS (APT)

  Fully qualified image URI (releasever must be set manually):
    registry.access.redhat.com/ubi9/ubi:9.4
    quay.io/centos/centos:stream9
    rockylinux:9
    almalinux:9

When --base is omitted a generic template with placeholder values is written.`,
		Example: `  distill init --base ubi9 --name myapp
  distill init --base debian --name myservice --destination myregistry.io/myservice:latest
  distill init --base ubi9 --variant dev --output dev.distill.yaml
  distill init --base registry.access.redhat.com/ubi9/ubi:9.4 --name myapp`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(name, base, variant, output, destination, force)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Image name (default: current directory name)")
	cmd.Flags().StringVarP(&base, "base", "b", "",
		"Base distribution (ubi9, ubi8, fedora, debian, ubuntu, ubuntu22)")
	cmd.Flags().StringVarP(&variant, "variant", "v", "runtime",
		`Image variant: "runtime" removes the package manager; "dev" retains it`)
	cmd.Flags().StringVarP(&output, "output", "o", "image.distill.yaml", "Output file path")
	cmd.Flags().StringVarP(&destination, "destination", "d", "",
		"Destination image reference (e.g. myregistry.io/myapp:latest)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing file")

	return cmd
}

func runInit(name, base, variant, output, destination string, force bool) error {
	if variant != "runtime" && variant != "dev" {
		return fmt.Errorf("variant must be \"runtime\" or \"dev\", got %q", variant)
	}

	// Resolve name from the current directory when not provided.
	if name == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		name = filepath.Base(cwd)
		name = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			if r >= 'A' && r <= 'Z' {
				return r + 32 // to lower
			}
			return '-'
		}, name)
	}

	// Check for existing file.
	if !force {
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("%s already exists — use --force to overwrite", output)
		}
	}

	content, err := spec.ScaffoldSpec(name, base, variant, destination)
	if err != nil {
		return err
	}

	if err := os.WriteFile(output, []byte(content), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", output, err)
	}

	fmt.Printf("Created %s\n", output)
	switch {
	case base == "":
		fmt.Println("Edit the file and replace placeholder values before running distill build.")
	case strings.ContainsAny(base, "/:"):
		// Fully qualified image URI — releasever needs to be set manually.
		if _, ok := spec.BasePresets[base]; !ok {
			fmt.Println("Set source.releasever to the distribution version (e.g. \"9\", \"bookworm\") before running distill build.")
		}
	}
	return nil
}
