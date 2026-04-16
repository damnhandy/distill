package builder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// DNFBuilder builds minimal OCI images for RPM-based distributions
// (RHEL, UBI, CentOS Stream, Rocky Linux, AlmaLinux, Fedora).
type DNFBuilder struct{}

// Build generates a multi-stage Dockerfile and runs it with the detected
// container CLI. The builder stage populates a /chroot directory using
// dnf --installroot inside the base image; the final stage is FROM scratch
// with the chroot copied in. Because the chroot lives on the container
// runtime's own overlay filesystem throughout, RPM scriptlets and hardlinks
// work correctly on all platforms without workarounds.
func (b *DNFBuilder) Build(ctx context.Context, s *spec.ImageSpec, platform string) error {
	cli := DetectCLI()

	contextDir, err := os.MkdirTemp("", "distill-dnf-*")
	if err != nil {
		return fmt.Errorf("creating build context: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(contextDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing build context: %v\n", err)
		}
	}()

	dockerfilePath := filepath.Join(contextDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dnfDockerfile(s)), 0o600); err != nil {
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

// dnfDockerfile generates the full multi-stage Dockerfile for a DNF-based image.
func dnfDockerfile(s *spec.ImageSpec) string {
	var b strings.Builder

	// ── Builder stage ────────────────────────────────────────────────────────

	fmt.Fprintf(&b, "FROM %s AS builder\n", s.Source.Image)

	b.WriteString("\n# Initialize a fresh RPM database and seed the repo configuration.\n")
	b.WriteString("RUN rpm --root /chroot --initdb \\\n")
	b.WriteString("    && mkdir -p /chroot/etc \\\n")
	b.WriteString("    && cp -r /etc/yum.repos.d /chroot/etc/\n")

	b.WriteString("\n# Install packages into the chroot.\n")
	b.WriteString("RUN dnf install -y -q \\\n")
	b.WriteString("    --installroot /chroot \\\n")
	fmt.Fprintf(&b, "    --releasever %s \\\n", s.Source.Releasever)
	b.WriteString("    --setopt=install_weak_deps=false \\\n")
	b.WriteString("    --setopt=tsflags=nodocs \\\n")
	b.WriteString("    --setopt=override_install_langs=en_US.utf8 \\\n")
	for i, pkg := range s.Contents.Packages {
		if i < len(s.Contents.Packages)-1 {
			fmt.Fprintf(&b, "    %s \\\n", pkg)
		} else {
			fmt.Fprintf(&b, "    %s\n", pkg)
		}
	}

	if s.Accounts != nil && (len(s.Accounts.Groups) > 0 || len(s.Accounts.Users) > 0) {
		b.WriteString("\n# Create groups and users inside the chroot.\n")
		b.WriteString("RUN ")
		first := true
		for _, g := range s.Accounts.Groups {
			if !first {
				b.WriteString(" \\\n    && ")
			}
			fmt.Fprintf(&b, "groupadd -R /chroot --gid %d %s", g.GID, g.Name)
			first = false
		}
		for _, u := range s.Accounts.Users {
			if !first {
				b.WriteString(" \\\n    && ")
			}
			shell := u.Shell
			if shell == "" {
				shell = "/sbin/nologin"
			}
			line := fmt.Sprintf("useradd -R /chroot --uid %d --gid %d -r -m -s %s", u.UID, u.GID, shell)
			if len(u.Groups) > 0 {
				line += " -G " + strings.Join(u.Groups, ",")
			}
			line += " " + u.Name
			b.WriteString(line)
			first = false
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "\nRUN dnf clean all --installroot /chroot --releasever %s\n", s.Source.Releasever)

	b.WriteString(pathsInstructions(s))

	if s.IsRuntime() {
		b.WriteString("\n# Remove the package manager for true immutability.\n")
		b.WriteString("# The RPM database is retained so 'rpm -qa' works for auditing.\n")
		b.WriteString("RUN rm -rf \\\n")
		b.WriteString("    /chroot/usr/bin/dnf* \\\n")
		b.WriteString("    /chroot/usr/bin/yum* \\\n")
		b.WriteString("    /chroot/usr/lib/python3*/site-packages/dnf \\\n")
		b.WriteString("    /chroot/usr/lib/python3*/site-packages/yum \\\n")
		b.WriteString("    /chroot/var/cache/dnf \\\n")
		b.WriteString("    /chroot/var/log/dnf* \\\n")
		b.WriteString("    /chroot/tmp/*\n")
	}

	// ── Final stage ──────────────────────────────────────────────────────────

	b.WriteString(scratchStageInstructions(s))

	return b.String()
}
