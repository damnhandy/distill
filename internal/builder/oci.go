package builder

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/damnhandy/distill/internal/spec"
)

// assemble creates a FROM scratch OCI image by adding the populated chroot
// directory into a buildah working container, applying OCI config from the
// spec, and committing with the given tag.
//
// buildah add (unlike buildah mount) does not require elevated privileges,
// so this step runs without root.
func assemble(ctx context.Context, s *spec.ImageSpec, chrootDir, tag, platform string) error {
	out := os.Stdout

	// Create a FROM scratch working container.
	containerID, err := capture(ctx, "buildah", "from", "--platform", platform, "scratch")
	if err != nil {
		return fmt.Errorf("creating scratch container: %w", err)
	}

	// Always clean up the working container, even on failure.
	defer func() {
		_ = run(ctx, io.Discard, "buildah", "rm", containerID)
	}()

	// Copy the entire chroot into the container root.
	if err := run(ctx, out, "buildah", "add", containerID, chrootDir, "/"); err != nil {
		return fmt.Errorf("adding chroot to container: %w", err)
	}

	// Apply environment variables.
	for k, v := range s.Image.Env {
		if err := run(ctx, out, "buildah", "config",
			"--env", fmt.Sprintf("%s=%s", k, v),
			containerID,
		); err != nil {
			return fmt.Errorf("setting env %s: %w", k, err)
		}
	}

	// Apply working directory.
	if s.Image.Workdir != "" {
		if err := run(ctx, out, "buildah", "config",
			"--workingdir", s.Image.Workdir,
			containerID,
		); err != nil {
			return fmt.Errorf("setting workdir: %w", err)
		}
	}

	// Apply default command.
	if len(s.Image.Cmd) > 0 {
		args := append([]string{"config"}, formatCmd(s.Image.Cmd)...)
		args = append(args, containerID)
		if err := run(ctx, out, "buildah", args...); err != nil {
			return fmt.Errorf("setting cmd: %w", err)
		}
	}

	// Run as the first defined user.
	if s.Accounts != nil && len(s.Accounts.Users) > 0 {
		u := s.Accounts.Users[0]
		if err := run(ctx, out, "buildah", "config",
			"--user", fmt.Sprintf("%d:%d", u.UID, u.GID),
			containerID,
		); err != nil {
			return fmt.Errorf("setting user: %w", err)
		}
	}

	// Apply OCI image title label.
	if err := run(ctx, out, "buildah", "config",
		"--label", fmt.Sprintf("org.opencontainers.image.title=%s", s.Name),
		containerID,
	); err != nil {
		return fmt.Errorf("setting image title label: %w", err)
	}

	// Commit the image.
	commitArgs := []string{"commit", containerID}
	if tag != "" {
		commitArgs = append(commitArgs, tag)
	}
	if err := run(ctx, out, "buildah", commitArgs...); err != nil {
		return fmt.Errorf("committing image: %w", err)
	}

	if tag != "" {
		fmt.Printf("\nBuilt %s\n", tag)
	}
	return nil
}

// formatCmd converts a string slice into buildah config --cmd arguments.
// buildah expects the command as individual --cmd flags or JSON array notation.
func formatCmd(cmd []string) []string {
	// Pass each element as a separate --cmd argument; buildah joins them.
	args := make([]string, 0, len(cmd)*2)
	for _, c := range cmd {
		args = append(args, "--cmd", c)
	}
	return args
}
