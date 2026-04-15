package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:     "distill",
	Short:   "Build minimal, immutable OCI images from enterprise Linux base distributions",
	Version: Version,
	Long: `distill builds minimal OCI images from a declarative ImageSpec YAML file.

Packages are installed into an isolated chroot directory inside a multi-stage
build container, then copied into a FROM scratch OCI image. The package manager
is never present in the final image — not removed as a layer, but never copied
in to begin with.

Supported distributions:
  RHEL / UBI (via DNF)
  Debian / Ubuntu (via APT + debootstrap)

Runtime requirements (detected automatically):
  macOS / Windows  — Docker Desktop (docker)
  Linux / WSL2     — Podman`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(
		newBuildCmd(),
		newScanCmd(),
		newAttestCmd(),
		newProvenanceCmd(),
		newVersionCmd(),
	)
}
