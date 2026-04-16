package builder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// APTBuilder builds minimal OCI images for Debian/Ubuntu distributions
// using debootstrap to create the initial rootfs.
type APTBuilder struct{}

// Build generates a multi-stage Dockerfile and runs it with the detected
// container CLI. The builder stage runs debootstrap inside the base image
// to populate /chroot; the final stage is FROM scratch with the chroot
// copied in.
func (b *APTBuilder) Build(ctx context.Context, s *spec.ImageSpec, platform string) error {
	cli := DetectCLI()

	contextDir, err := os.MkdirTemp("", "distill-apt-*")
	if err != nil {
		return fmt.Errorf("creating build context: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(contextDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing build context: %v\n", err)
		}
	}()

	dockerfilePath := filepath.Join(contextDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(aptDockerfile(s)), 0o600); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	args := []string{"build", "--platform", platform, "-f", dockerfilePath}
	if s.Destination != nil && s.Destination.Image != "" {
		args = append(args, "-t", s.Destination.Ref())
	}
	args = append(args, contextDir)

	if err := run(ctx, os.Stdout, string(cli), args...); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if s.Destination != nil && s.Destination.Image != "" {
		fmt.Printf("\nBuilt %s\n", s.Destination.Ref())
	}
	return nil
}

// aptDockerfile generates the full multi-stage Dockerfile for an APT-based image.
func aptDockerfile(s *spec.ImageSpec) string {
	var b strings.Builder

	// ── Builder stage ────────────────────────────────────────────────────────

	fmt.Fprintf(&b, "FROM %s AS builder\n", s.Source.Image)

	b.WriteString("\n# Install debootstrap if not present in the base image.\n")
	b.WriteString("RUN apt-get update -qq \\\n")
	b.WriteString("    && apt-get install -y -qq debootstrap\n")

	b.WriteString("\n# Bootstrap a minimal rootfs.\n")
	b.WriteString("# --variant=minbase installs only Essential:yes packages plus the explicit list.\n")
	fmt.Fprintf(&b, "RUN debootstrap --variant=minbase --include=%s %s /chroot\n",
		strings.Join(s.Contents.Packages, ","),
		s.Source.Releasever,
	)

	if s.Accounts != nil && (len(s.Accounts.Groups) > 0 || len(s.Accounts.Users) > 0) {
		b.WriteString("\n# Create groups and users inside the chroot.\n")
		b.WriteString("RUN ")
		first := true
		for _, g := range s.Accounts.Groups {
			if !first {
				b.WriteString(" \\\n    && ")
			}
			fmt.Fprintf(&b, "chroot /chroot groupadd --gid %d %s", g.GID, g.Name)
			first = false
		}
		for _, u := range s.Accounts.Users {
			if !first {
				b.WriteString(" \\\n    && ")
			}
			shell := u.Shell
			if shell == "" {
				shell = "/usr/sbin/nologin"
			}
			line := fmt.Sprintf("chroot /chroot useradd --uid %d --gid %d -r -m -s %s", u.UID, u.GID, shell)
			if len(u.Groups) > 0 {
				line += " -G " + strings.Join(u.Groups, ",")
			}
			line += " " + u.Name
			b.WriteString(line)
			first = false
		}
		b.WriteString("\n")
	}

	b.WriteString("\n# Remove APT package lists and cache.\n")
	b.WriteString("RUN rm -rf \\\n")
	b.WriteString("    /chroot/var/cache/apt/archives/*.deb \\\n")
	b.WriteString("    /chroot/var/lib/apt/lists/* \\\n")
	b.WriteString("    /chroot/tmp/*\n")

	b.WriteString(pathsInstructions(s))

	if s.IsRuntime() {
		b.WriteString("\n# Remove apt and dpkg for true immutability.\n")
		b.WriteString("RUN chroot /chroot dpkg --purge --force-depends apt apt-utils 2>/dev/null || true \\\n")
		b.WriteString("    && rm -rf \\\n")
		b.WriteString("        /chroot/usr/bin/apt* \\\n")
		b.WriteString("        /chroot/usr/bin/dpkg* \\\n")
		b.WriteString("        /chroot/var/lib/dpkg/info/*.list\n")
	}

	// ── Final stage ──────────────────────────────────────────────────────────

	b.WriteString(scratchStageInstructions(s))

	return b.String()
}
