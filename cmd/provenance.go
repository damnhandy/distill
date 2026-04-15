package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/damnhandy/distill/internal/builder"
)

func newProvenanceCmd() *cobra.Command {
	var (
		specPath      string
		predicatePath string
	)

	cmd := &cobra.Command{
		Use:   "provenance <image>",
		Short: "Attach a SLSA v0.2 provenance attestation to an image",
		Long: `Provenance generates a SLSA v0.2 provenance predicate describing how the
image was built and attaches it as a cosign attestation using keyless signing
(Sigstore). The attestation is stored in the image's registry alongside the image.

When --spec is provided, the predicate is enriched with:
  - configSource: the spec file URI and its SHA-256 digest
  - materials:    the base image reference and its registry digest
  - parameters:   the base image reference

Attestations can be verified with:
  cosign verify-attestation --type slsaprovenance <image>
  slsa-verifier verify-image <image> --source-uri github.com/damnhandy/distill`,
		Args: cobra.ExactArgs(1),
		Example: `  distill provenance myregistry.io/rhel9-runtime:latest
  distill provenance --spec examples/rhel9-runtime/image.yaml myregistry.io/rhel9-runtime:latest
  distill provenance --spec image.yaml --predicate provenance.json myregistry.io/rhel9-runtime:latest`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProvenance(cmd.Context(), args[0], specPath, predicatePath)
		},
	}

	cmd.Flags().StringVarP(&specPath, "spec", "s", "", "Path to the image.yaml spec used during the build (enriches provenance)")
	cmd.Flags().StringVarP(&predicatePath, "predicate", "p", "", "Write the predicate JSON to this path (default: temp file)")

	return cmd
}

func runProvenance(ctx context.Context, image, specPath, predicatePath string) error {
	return builder.Provenance(ctx, image, builder.ProvenanceOptions{
		SpecPath:      specPath,
		PredicatePath: predicatePath,
	})
}
