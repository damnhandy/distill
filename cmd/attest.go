package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/builder"
)

func newAttestCmd() *cobra.Command {
	var (
		outputPath string
		specPath   string
	)

	cmd := &cobra.Command{
		Use:   "attest [<image>]",
		Short: "Generate an SPDX SBOM for an image using Syft",
		Long: `Attest generates a Software Bill of Materials (SBOM) in SPDX JSON format
for a container image using Anchore Syft.

The SBOM captures every package present in the image, including exact RPM or
dpkg NVRs, making it suitable for compliance and vulnerability auditing.

The image reference can be supplied in two ways:

  1. As a positional argument:
       distill attest ghcr.io/myorg/myapp:latest

  2. Via --spec, which reads Tags[0] from the spec file:
       distill attest --spec image.distill.yaml

When both are provided the positional argument takes precedence.`,
		Args: cobra.RangeArgs(0, 1),
		Example: `  distill attest myregistry.io/rhel9-runtime:latest
  distill attest --spec examples/rhel9-runtime/image.distill.yaml
  distill attest --output sbom.spdx.json myregistry.io/rhel9-runtime:latest
  distill attest --spec image.distill.yaml --output sbom.spdx.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			image, err := resolveImage(cmd.Name(), args, specPath)
			if err != nil {
				return err
			}
			return runAttest(cmd.Context(), image, outputPath)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "sbom.spdx.json",
		"Output file path for the SBOM")
	cmd.Flags().StringVarP(&specPath, "spec", "s", "",
		"Path to the .distill.yaml spec file (resolves the image reference from Tags[0])")

	return cmd
}

func runAttest(ctx context.Context, image, outputPath string) error {
	fmt.Printf("Generating SBOM for %q -> %s ...\n\n", image, outputPath)
	return builder.Attest(ctx, image, outputPath)
}
