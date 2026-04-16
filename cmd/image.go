package cmd

import (
	"fmt"
	"os"

	"github.com/damnhandy/distill/internal/spec"
)

// resolveImage determines the OCI image reference to operate on.
//
// Resolution order (explicit beats implicit):
//  1. args[0] — positional argument provided by the user.
//  2. specPath — parse the spec file and use destination.Ref().
//  3. neither — return an error explaining both options.
//
// When both are provided, the positional argument takes precedence.
func resolveImage(cmdName string, args []string, specPath string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if specPath != "" {
		data, err := os.ReadFile(specPath) //nolint:gosec // G304: specPath is a user-supplied CLI argument
		if err != nil {
			return "", fmt.Errorf("reading spec %q: %w", specPath, err)
		}
		s, err := spec.Parse(data)
		if err != nil {
			return "", err
		}
		if s.Destination == nil || s.Destination.Image == "" {
			return "", fmt.Errorf(
				"spec %q has no destination defined — add a 'destination.image' entry or pass the image reference as a positional argument",
				specPath,
			)
		}
		return s.Destination.Ref(), nil
	}
	return "", fmt.Errorf(
		"no image reference provided: pass the image as a positional argument or use --spec\n\nExamples:\n  distill %s <image>\n  distill %s --spec image.distill.yaml",
		cmdName, cmdName,
	)
}
