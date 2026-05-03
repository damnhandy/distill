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

func newPublishCmd() *cobra.Command {
	var (
		specFile         string
		platformOverride string
		skipBuild        bool
		skipPipeline     bool
	)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Build, push, and run the supply-chain pipeline for an image",
		Long: `publish executes the full deployment workflow in order:

  1. Build all platforms declared in the spec (skipped with --skip-build).
  2. Scan for CVEs (if pipeline.scan.enabled) — fails before pushing.
  3. Push the image to the registry.
  4. Generate SBOM (if pipeline.sbom.enabled).
  5. Attach SLSA provenance (if pipeline.provenance.enabled).

Steps 2, 4, and 5 are controlled by the pipeline section of the spec file.
Use --skip-pipeline to push only, without running any pipeline steps.

A destination.image entry in the spec is required.

Container runtime is selected automatically based on the host OS:
  macOS / Windows  — docker
  Linux / WSL2     — podman`,
		Example: `  distill publish --spec image.distill.yaml
  distill publish --spec image.distill.yaml --platform linux/amd64
  distill publish --spec image.distill.yaml --skip-build
  distill publish --spec image.distill.yaml --skip-pipeline`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPublish(cmd.Context(), specFile, platformOverride, skipBuild, skipPipeline)
		},
	}

	cmd.Flags().StringVarP(&specFile, "spec", "s", "", "Path to the .distill.yaml spec file (required)")
	cmd.Flags().StringVar(&platformOverride, "platform", "", "Override platform (e.g. linux/arm64); builds/pushes only this platform")
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "Skip the build step (assumes the image is already built locally)")
	cmd.Flags().BoolVar(&skipPipeline, "skip-pipeline", false, "Skip all pipeline steps; push only")
	_ = cmd.MarkFlagRequired("spec")

	return cmd
}

func runPublish(ctx context.Context, specFile, platformOverride string, skipBuild, skipPipeline bool) error {
	data, err := os.ReadFile(specFile) //nolint:gosec // G304: specFile is a CLI argument provided by the operator
	if err != nil {
		return fmt.Errorf("reading spec %q: %w", specFile, err)
	}

	imageSpec, err := spec.Parse(data)
	if err != nil {
		return err
	}

	if imageSpec.Destination == nil || imageSpec.Destination.Image == "" {
		return fmt.Errorf("publish requires destination.image to be set in the spec")
	}

	if err := builder.CheckDeps(); err != nil {
		return err
	}

	platforms := imageSpec.EffectivePlatforms()
	if platformOverride != "" {
		platforms = []string{platformOverride}
	}

	image := imageSpec.Destination.Ref()

	// Step 1: Build
	if !skipBuild {
		b, err := builder.New(imageSpec.Source.PackageManager)
		if err != nil {
			return err
		}
		fmt.Printf("Building %q\n  source:    %s\n  variant:   %s\n  platforms: %v\n  packages:  %d\n\n",
			imageSpec.Name, imageSpec.Source.Image, imageSpec.Variant, platforms, len(imageSpec.Contents.Packages))
		opts := builder.BuildOptions{SourceDir: filepath.Dir(specFile)}
		for _, platform := range platforms {
			if err := b.Build(ctx, imageSpec, platform, opts); err != nil {
				return err
			}
		}
	}

	// Step 2: Pre-push scan (fail fast before pushing a vulnerable image)
	if !skipPipeline {
		if err := builder.RunPipeline(ctx, imageSpec, image, specFile, builder.PipelineModeLocal); err != nil {
			return err
		}
	}

	// Step 3: Push
	fmt.Printf("\n── Push ──────────────────────────────────────────────────\n")
	if err := builder.Push(ctx, image); err != nil {
		return err
	}

	// Step 4 + 5: Post-push pipeline (sbom + provenance)
	if !skipPipeline {
		if err := builder.RunPipeline(ctx, imageSpec, image, specFile, builder.PipelineModePublish); err != nil {
			return err
		}
	}

	return nil
}
