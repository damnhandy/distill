package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/builder"
	"github.com/damnhandy/distill/internal/spec"
)

func newBuildCmd() *cobra.Command {
	var (
		specFile         string
		platformOverride string
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build minimal OCI images from a .distill.yaml spec file",
		Long: `Build reads a declarative .distill.yaml spec file and produces minimal OCI
images using a chroot bootstrap strategy.

The build runs inside a privileged container using the base image specified in
the spec, so the correct package manager, repo configuration, and release
version are always available. The populated chroot is then committed as a
FROM scratch image.

Target platforms and destination image are declared in the spec file:

  platforms:
    - linux/amd64
    - linux/arm64
  destination:
    image: myregistry.io/myapp
    releasever: latest

Container runtime is selected automatically based on the host OS:
  macOS / Windows  — docker build
  Linux / WSL2     — podman build

The --platform flag overrides the spec's platforms list and builds only
the specified platform.`,
		Example: `  distill build --spec examples/rhel9-runtime/image.distill.yaml
  distill build --spec image.distill.yaml --platform linux/arm64`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBuild(cmd.Context(), specFile, platformOverride)
		},
	}

	cmd.Flags().StringVarP(&specFile, "spec", "s", "", "Path to the .distill.yaml spec file (required)")
	cmd.Flags().StringVar(&platformOverride, "platform", "", "Override platform (e.g. linux/arm64); builds only this platform")
	_ = cmd.MarkFlagRequired("spec")

	return cmd
}

func runBuild(ctx context.Context, specFile, platformOverride string) error {
	data, err := os.ReadFile(specFile) //nolint:gosec // G304: specFile is a CLI argument provided by the operator
	if err != nil {
		return fmt.Errorf("reading spec %q: %w", specFile, err)
	}

	imageSpec, err := spec.Parse(data)
	if err != nil {
		return err
	}

	if err := builder.CheckDeps(); err != nil {
		return err
	}

	b, err := builder.New(imageSpec.Source.PackageManager)
	if err != nil {
		return err
	}

	platforms := imageSpec.EffectivePlatforms()
	if platformOverride != "" {
		platforms = []string{platformOverride}
	}

	fmt.Printf("Building %q\n  source:    %s\n  variant:   %s\n  platforms: %v\n  packages:  %d\n\n",
		imageSpec.Name, imageSpec.Source.Image, imageSpec.Variant, platforms, len(imageSpec.Contents.Packages))

	for _, platform := range platforms {
		if err := b.Build(ctx, imageSpec, platform); err != nil {
			return err
		}
	}

	return nil
}
