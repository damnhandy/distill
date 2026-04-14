// Package builder implements the chroot bootstrap build strategy for
// producing minimal OCI images from enterprise Linux base distributions.
package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/damnhandy/distill/internal/spec"
)

// Builder builds an OCI image from an ImageSpec.
type Builder interface {
	Build(ctx context.Context, s *spec.ImageSpec, tag, platform string) error
}

// New returns the Builder for the given package manager.
func New(packageManager string) (Builder, error) {
	switch packageManager {
	case "dnf":
		return &DNFBuilder{}, nil
	case "apt":
		return &APTBuilder{}, nil
	default:
		return nil, fmt.Errorf("unsupported package manager %q — supported: dnf, apt", packageManager)
	}
}

// CheckDeps verifies that the required runtime tools are available on PATH.
func CheckDeps() error {
	required := []string{"podman", "buildah"}
	var missing []string
	for _, bin := range required {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tools not found on PATH: %v\n"+
			"Install them with: devbox shell", missing)
	}
	return nil
}

// Scan runs Grype against image and fails on findings at or above failOn severity.
func Scan(ctx context.Context, image, failOn string) error {
	if _, err := exec.LookPath("grype"); err != nil {
		return fmt.Errorf("grype not found on PATH — install with: devbox shell")
	}
	return run(ctx, os.Stdout,
		"grype", image,
		"--fail-on", failOn,
		"--output", "table",
	)
}

// Attest generates an SPDX SBOM for image using Syft and writes it to outputPath.
func Attest(ctx context.Context, image, outputPath string) error {
	if _, err := exec.LookPath("syft"); err != nil {
		return fmt.Errorf("syft not found on PATH — install with: devbox shell")
	}
	return run(ctx, os.Stdout,
		"syft", image,
		"--output", fmt.Sprintf("spdx-json=%s", outputPath),
	)
}
