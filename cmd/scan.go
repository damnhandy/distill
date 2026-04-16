package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/builder"
)

func newScanCmd() *cobra.Command {
	var (
		failOn   string
		specPath string
	)

	cmd := &cobra.Command{
		Use:   "scan [<image>]",
		Short: "Scan an image for CVEs using Grype",
		Long: `Scan runs an Anchore Grype vulnerability scan against a container image.
The scan fails if any findings are at or above the specified severity level.

The image reference can be supplied in two ways:

  1. As a positional argument:
       distill scan ghcr.io/myorg/myapp:latest

  2. Via --spec, which reads Tags[0] from the spec file:
       distill scan --spec image.distill.yaml

When both are provided the positional argument takes precedence.`,
		Args: cobra.RangeArgs(0, 1),
		Example: `  distill scan myregistry.io/rhel9-runtime:latest
  distill scan --spec examples/rhel9-runtime/image.distill.yaml
  distill scan --fail-on high myregistry.io/rhel9-runtime:latest
  distill scan --spec image.distill.yaml --fail-on high`,
		RunE: func(cmd *cobra.Command, args []string) error {
			image, err := resolveImage(cmd.Name(), args, specPath)
			if err != nil {
				return err
			}
			return runScan(cmd.Context(), image, failOn)
		},
	}

	cmd.Flags().StringVar(&failOn, "fail-on", "critical",
		"Minimum severity that fails the scan (critical, high, medium, low, negligible)")
	cmd.Flags().StringVarP(&specPath, "spec", "s", "",
		"Path to the .distill.yaml spec file (resolves the image reference from Tags[0])")

	return cmd
}

func runScan(ctx context.Context, image, failOn string) error {
	fmt.Printf("Scanning %q (fail-on: %s) ...\n\n", image, failOn)
	return builder.Scan(ctx, image, failOn)
}
