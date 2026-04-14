package cmd

import (
	"context"
	"fmt"

	"github.com/damnhandy/distill/internal/builder"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var failOn string

	cmd := &cobra.Command{
		Use:   "scan <image>",
		Short: "Scan an image for CVEs using Grype",
		Long: `Scan runs an Anchore Grype vulnerability scan against a container image.
The scan fails if any findings are at or above the specified severity level.`,
		Args: cobra.ExactArgs(1),
		Example: `  distill scan myregistry.io/rhel9-runtime:latest
  distill scan --fail-on high myregistry.io/rhel9-runtime:latest`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.Context(), args[0], failOn)
		},
	}

	cmd.Flags().StringVar(&failOn, "fail-on", "critical",
		"Minimum severity that fails the scan (critical, high, medium, low, negligible)")

	return cmd
}

func runScan(ctx context.Context, image, failOn string) error {
	fmt.Printf("Scanning %q (fail-on: %s) ...\n\n", image, failOn)
	return builder.Scan(ctx, image, failOn)
}
