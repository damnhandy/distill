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

	// Suppress debconf prompts during debootstrap. Without this, packages such
	// as tzdata hang indefinitely waiting for timezone input on Ubuntu images.
	b.WriteString("ENV DEBIAN_FRONTEND=noninteractive\n")

	b.WriteString("\n# Install debootstrap if not present in the base image.\n")
	b.WriteString("RUN apt-get update -qq \\\n")
	b.WriteString("    && apt-get install -y -qq debootstrap\n")

	// Ubuntu's Essential package set includes init-system-helpers, which calls
	// invoke-rc.d in its postinst and hangs inside a Docker build chroot without
	// a policy-rc.d guard. We use --foreign to split debootstrap into two stages
	// so we can inject policy-rc.d before the configure scripts run. This is also
	// the correct approach for Debian, so we apply it uniformly.
	b.WriteString("\n# Stage 1: download and unpack packages only (no postinst scripts yet).\n")
	fmt.Fprintf(&b, "RUN debootstrap --foreign --variant=minbase --include=%s \\\n",
		strings.Join(s.Contents.Packages, ","))
	fmt.Fprintf(&b, "    %s /chroot %s\n", s.Source.Releasever, aptMirror(s.Source.Image))

	// Ubuntu's init-system-helpers postinst calls multiple service management
	// tools that hang inside a Docker build chroot:
	//   - invoke-rc.d  → blocked by policy-rc.d
	//   - deb-systemd-helper / deb-systemd-invoke → must be mocked directly
	//   - systemctl    → must be mocked (not present in minbase, but searched)
	// Replace all of them with no-op stubs before Stage 2 runs.
	b.WriteString("\n# Stub out service/init tools to prevent hangs during Stage 2.\n")
	b.WriteString("RUN printf '#!/bin/sh\\nexit 101\\n' > /chroot/usr/sbin/policy-rc.d \\\n")
	b.WriteString("    && printf '#!/bin/sh\\nexit 0\\n'  > /chroot/usr/bin/deb-systemd-helper \\\n")
	b.WriteString("    && printf '#!/bin/sh\\nexit 0\\n'  > /chroot/usr/bin/deb-systemd-invoke \\\n")
	b.WriteString("    && printf '#!/bin/sh\\nexit 0\\n'  > /chroot/usr/bin/systemctl \\\n")
	b.WriteString("    && chmod +x \\\n")
	b.WriteString("        /chroot/usr/sbin/policy-rc.d \\\n")
	b.WriteString("        /chroot/usr/bin/deb-systemd-helper \\\n")
	b.WriteString("        /chroot/usr/bin/deb-systemd-invoke \\\n")
	b.WriteString("        /chroot/usr/bin/systemctl\n")

	b.WriteString("\n# Stage 2: run postinst scripts inside the chroot.\n")
	b.WriteString("RUN chroot /chroot /debootstrap/debootstrap --second-stage\n")

	b.WriteString("\n# Remove all stubs — none belong in the final runtime image.\n")
	b.WriteString("RUN rm -f \\\n")
	b.WriteString("    /chroot/usr/sbin/policy-rc.d \\\n")
	b.WriteString("    /chroot/usr/bin/deb-systemd-helper \\\n")
	b.WriteString("    /chroot/usr/bin/deb-systemd-invoke \\\n")
	b.WriteString("    /chroot/usr/bin/systemctl\n")

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

// aptMirror returns the canonical package archive URL for the given base image.
// debootstrap requires an explicit mirror when the host and target distro differ,
// and Ubuntu requires its own archive even when the host is Ubuntu.
func aptMirror(image string) string {
	if strings.Contains(image, "ubuntu") {
		return "http://archive.ubuntu.com/ubuntu"
	}
	return "http://deb.debian.org/debian"
}
