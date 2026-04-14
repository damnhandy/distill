package cmd

import (
	"context"
	"fmt"

	"github.com/damnhandy/distill/internal/builder"
	"github.com/spf13/cobra"
)

func newAttestCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "attest <image>",
		Short: "Generate an SPDX SBOM for an image using Syft",
		Long: `Attest generates a Software Bill of Materials (SBOM) in SPDX JSON format
for a container image using Anchore Syft.

The SBOM captures every package present in the image, including exact RPM or
dpkg NVRs, making it suitable for compliance and vulnerability auditing.`,
		Args: cobra.ExactArgs(1),
		Example: `  distill attest myregistry.io/rhel9-runtime:latest
  distill attest --output sbom.spdx.json myregistry.io/rhel9-runtime:latest`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttest(cmd.Context(), args[0], outputPath)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "sbom.spdx.json",
		"Output file path for the SBOM")

	return cmd
}

func runAttest(ctx context.Context, image, outputPath string) error {
	fmt.Printf("Generating SBOM for %q -> %s ...\n\n", image, outputPath)
	return builder.Attest(ctx, image, outputPath)
}
