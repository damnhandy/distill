package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/damnhandy/distill/internal/builder"
	"github.com/damnhandy/distill/internal/spec"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	var (
		specFile string
		tag      string
		platform string
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a minimal OCI image from an ImageSpec YAML file",
		Long: `Build reads a declarative ImageSpec YAML file and produces a minimal OCI
image using a chroot bootstrap strategy.

The build runs inside a privileged container (podman run --privileged) using
the base image specified in the spec, so the correct package manager, repo
configuration, and release version are always available. The populated chroot
is then committed as a FROM scratch image via buildah.`,
		Example: `  distill build --spec examples/rhel9-runtime/image.yaml --tag myregistry.io/rhel9-runtime:latest
  distill build --spec examples/debian-runtime/image.yaml --tag myregistry.io/debian-runtime:latest
  distill build --spec image.yaml --tag myregistry.io/my-app:1.0.0 --platform linux/arm64`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd.Context(), specFile, tag, platform)
		},
	}

	cmd.Flags().StringVarP(&specFile, "spec", "s", "", "Path to the ImageSpec YAML file (required)")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Image tag for the output image (e.g. registry/image:tag)")
	cmd.Flags().StringVar(&platform, "platform", "linux/amd64", "Target platform (linux/amd64, linux/arm64)")
	_ = cmd.MarkFlagRequired("spec")

	return cmd
}

func runBuild(ctx context.Context, specFile, tag, platform string) error {
	data, err := os.ReadFile(specFile)
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

	b, err := builder.New(imageSpec.Base.PackageManager)
	if err != nil {
		return err
	}

	fmt.Printf("Building %q\n  base:     %s\n  platform: %s\n  packages: %d\n\n",
		imageSpec.Name, imageSpec.Base.Image, platform, len(imageSpec.Packages))

	return b.Build(ctx, imageSpec, tag, platform)
}
