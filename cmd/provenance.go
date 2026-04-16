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
		Use:   "provenance [<image>]",
		Short: "Attach a SLSA v0.2 provenance attestation to an image",
		Long: `Provenance generates a SLSA v0.2 provenance predicate describing how the
image was built and attaches it as a cosign attestation using keyless signing
(Sigstore). The attestation is stored in the image's registry alongside the image.

The image reference can be supplied in two ways:

  1. As a positional argument:
       distill provenance ghcr.io/myorg/myapp:latest

  2. Via --spec, which reads Tags[0] from the spec file:
       distill provenance --spec image.distill.yaml

When both are provided the positional argument takes precedence.

When --spec is provided, the predicate is enriched with:
  - configSource: the spec file URI and its SHA-256 digest
  - materials:    the base image reference and its registry digest
  - parameters:   the base image reference

Attestations can be verified with:
  cosign verify-attestation --type slsaprovenance <image>
  slsa-verifier verify-image <image> --source-uri github.com/damnhandy/distill`,
		Args: cobra.RangeArgs(0, 1),
		Example: `  distill provenance myregistry.io/rhel9-runtime:latest
  distill provenance --spec image.distill.yaml
  distill provenance --spec examples/rhel9-runtime/image.distill.yaml myregistry.io/rhel9-runtime:latest
  distill provenance --spec image.distill.yaml --predicate provenance.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			image, err := resolveImage(cmd.Name(), args, specPath)
			if err != nil {
				return err
			}
			return runProvenance(cmd.Context(), image, specPath, predicatePath)
		},
	}

	cmd.Flags().StringVarP(&specPath, "spec", "s", "", "Path to the .distill.yaml spec file (enriches provenance; resolves image reference from Tags[0])")
	cmd.Flags().StringVarP(&predicatePath, "predicate", "p", "", "Write the predicate JSON to this path (default: temp file)")

	return cmd
}

func runProvenance(ctx context.Context, image, specPath, predicatePath string) error {
	return builder.Provenance(ctx, image, builder.ProvenanceOptions{
		SpecPath:      specPath,
		PredicatePath: predicatePath,
	})
}
