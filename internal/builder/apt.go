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

// Build bootstraps a chroot using debootstrap inside a privileged container,
// then assembles the result into a FROM scratch OCI image.
func (b *APTBuilder) Build(ctx context.Context, s *spec.ImageSpec, tag, platform string) error {
	workDir, err := os.MkdirTemp("", "distill-apt-*")
	if err != nil {
		return fmt.Errorf("creating workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	chrootDir := filepath.Join(workDir, "chroot")
	if err := os.MkdirAll(chrootDir, 0o755); err != nil {
		return fmt.Errorf("creating chroot dir: %w", err)
	}

	scriptPath := filepath.Join(workDir, "bootstrap.sh")
	if err := os.WriteFile(scriptPath, []byte(aptBootstrapScript(s)), 0o755); err != nil {
		return fmt.Errorf("writing bootstrap script: %w", err)
	}

	fmt.Println("Running APT bootstrap (this may take a minute)...")
	if err := run(ctx, os.Stdout, "podman", "run",
		"--rm",
		"--privileged",
		"--platform", platform,
		"-v", workDir+":/workspace:z",
		s.Base.Image,
		"/bin/bash", "/workspace/bootstrap.sh",
	); err != nil {
		return fmt.Errorf("APT bootstrap failed: %w", err)
	}

	fmt.Println("Assembling final image...")
	return assemble(ctx, s, chrootDir, tag, platform)
}

// aptBootstrapScript generates the shell script that runs inside the privileged
// Debian/Ubuntu build container.
func aptBootstrapScript(s *spec.ImageSpec) string {
	var b strings.Builder

	b.WriteString("#!/bin/bash\n")
	b.WriteString("set -euxo pipefail\n\n")
	b.WriteString("CHROOT=/workspace/chroot\n\n")

	b.WriteString("# Install debootstrap if not present in the base image.\n")
	b.WriteString("apt-get update -qq\n")
	b.WriteString("apt-get install -y -qq debootstrap\n\n")

	b.WriteString("# Bootstrap a minimal Debian/Ubuntu rootfs.\n")
	b.WriteString("# --variant=minbase installs only Essential:yes packages\n")
	b.WriteString("# plus the explicit package list.\n")
	b.WriteString(fmt.Sprintf(
		"debootstrap --variant=minbase --include=%s %s \"$CHROOT\"\n\n",
		strings.Join(s.Packages, ","),
		s.Base.Releasever,
	))

	if s.Accounts != nil {
		for _, g := range s.Accounts.Groups {
			b.WriteString(fmt.Sprintf(
				"chroot \"$CHROOT\" groupadd --gid %d %s\n", g.GID, g.Name,
			))
		}
		for _, u := range s.Accounts.Users {
			shell := u.Shell
			if shell == "" {
				shell = "/usr/sbin/nologin"
			}
			line := fmt.Sprintf(
				"chroot \"$CHROOT\" useradd --uid %d --gid %d -r -m -s %s",
				u.UID, u.GID, shell,
			)
			if len(u.Groups) > 0 {
				line += " -G " + strings.Join(u.Groups, ",")
			}
			line += " " + u.Name
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("# Remove APT package lists and cache.\n")
	b.WriteString("rm -rf \\\n")
	b.WriteString("    \"$CHROOT\"/var/cache/apt/archives/*.deb \\\n")
	b.WriteString("    \"$CHROOT\"/var/lib/apt/lists/* \\\n")
	b.WriteString("    \"$CHROOT/tmp/\"*\n\n")

	if s.IsImmutable() {
		b.WriteString("# Remove apt and dpkg for true immutability.\n")
		b.WriteString("chroot \"$CHROOT\" dpkg --purge --force-depends apt apt-utils 2>/dev/null || true\n")
		b.WriteString("rm -rf \\\n")
		b.WriteString("    \"$CHROOT\"/usr/bin/apt* \\\n")
		b.WriteString("    \"$CHROOT\"/usr/bin/dpkg* \\\n")
		b.WriteString("    \"$CHROOT\"/var/lib/dpkg/info/*.list\n")
	}

	return b.String()
}
