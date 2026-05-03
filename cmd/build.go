package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/builder"
	"github.com/damnhandy/distill/internal/spec"
)

func newBuildCmd() *cobra.Command {
	var (
		specFile         string
		platformOverride string
		runPipeline      bool
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
the specified platform.

Use --pipeline to run the supply-chain steps declared in the spec's pipeline
section (scan, sbom) after the build completes. These steps operate on the
locally-built image and do not require a registry push. Provenance attestation
requires a push and runs only via "distill publish".`,
		Example: `  distill build --spec examples/rhel9-runtime/image.distill.yaml
  distill build --spec image.distill.yaml --platform linux/arm64
  distill build --spec image.distill.yaml --pipeline`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBuild(cmd.Context(), specFile, platformOverride, runPipeline)
		},
	}

	cmd.Flags().StringVarP(&specFile, "spec", "s", "", "Path to the .distill.yaml spec file (required)")
	cmd.Flags().StringVar(&platformOverride, "platform", "", "Override platform (e.g. linux/arm64); builds only this platform")
	cmd.Flags().BoolVar(&runPipeline, "pipeline", false, "Run pipeline steps (scan, sbom) defined in the spec after building")
	_ = cmd.MarkFlagRequired("spec")

	return cmd
}

func runBuild(ctx context.Context, specFile, platformOverride string, pipeline bool) error {
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

	opts := builder.BuildOptions{SourceDir: filepath.Dir(specFile)}
	for _, platform := range platforms {
		if err := b.Build(ctx, imageSpec, platform, opts); err != nil {
			return err
		}
	}

	if pipeline {
		if imageSpec.Destination == nil || imageSpec.Destination.Image == "" {
			return fmt.Errorf("--pipeline requires destination.image to be set in the spec")
		}
		if err := builder.RunPipeline(ctx, imageSpec, imageSpec.Destination.Ref(), specFile, builder.PipelineModeLocal); err != nil {
			return err
		}
	}

	return nil
}
