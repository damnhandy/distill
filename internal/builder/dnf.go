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

// Build bootstraps a chroot using DNF inside a privileged container, then
// assembles the result into a FROM scratch OCI image.
func (b *DNFBuilder) Build(ctx context.Context, s *spec.ImageSpec, tag, platform string) error {
	// Temporary workspace on the host — the chroot directory is bind-mounted
	// into the build container so its contents land on the host after the
	// container exits.
	workDir, err := os.MkdirTemp("", "distill-dnf-*")
	if err != nil {
		return fmt.Errorf("creating workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	chrootDir := filepath.Join(workDir, "chroot")
	if err := os.MkdirAll(chrootDir, 0o755); err != nil {
		return fmt.Errorf("creating chroot dir: %w", err)
	}

	scriptPath := filepath.Join(workDir, "bootstrap.sh")
	if err := os.WriteFile(scriptPath, []byte(dnfBootstrapScript(s)), 0o755); err != nil {
		return fmt.Errorf("writing bootstrap script: %w", err)
	}

	// Run the bootstrap inside a privileged instance of the base image.
	// Using the base image ensures the correct DNF version, repo config,
	// and releasever metadata are always available.
	fmt.Println("Running DNF bootstrap (this may take a minute)...")
	if err := run(ctx, os.Stdout, "podman", "run",
		"--rm",
		"--privileged",
		"--platform", platform,
		"-v", workDir+":/workspace:z",
		s.Base.Image,
		"/bin/bash", "/workspace/bootstrap.sh",
	); err != nil {
		return fmt.Errorf("DNF bootstrap failed: %w", err)
	}

	fmt.Println("Assembling final image...")
	return assemble(ctx, s, chrootDir, tag, platform)
}

// dnfBootstrapScript generates the shell script that runs inside the privileged
// build container to populate the chroot directory.
func dnfBootstrapScript(s *spec.ImageSpec) string {
	var b strings.Builder

	b.WriteString("#!/bin/bash\n")
	b.WriteString("set -euxo pipefail\n\n")
	b.WriteString("CHROOT=/workspace/chroot\n\n")

	b.WriteString("# Initialize a fresh RPM database inside the chroot.\n")
	b.WriteString("rpm --root \"$CHROOT\" --initdb\n\n")

	b.WriteString("# Seed the host repo configuration so DNF resolves against\n")
	b.WriteString("# the correct repos for this releasever.\n")
	b.WriteString("mkdir -p \"$CHROOT/etc\"\n")
	b.WriteString("cp -r /etc/yum.repos.d \"$CHROOT/etc/\"\n\n")

	b.WriteString("# Install packages.\n")
	b.WriteString("dnf install -y -q \\\n")
	b.WriteString("    --installroot \"$CHROOT\" \\\n")
	b.WriteString(fmt.Sprintf("    --releasever %s \\\n", s.Base.Releasever))
	b.WriteString("    --setopt=install_weak_deps=false \\\n")
	b.WriteString("    --setopt=tsflags=nodocs \\\n")
	b.WriteString("    --setopt=override_install_langs=en_US.utf8 \\\n")
	for i, pkg := range s.Packages {
		if i < len(s.Packages)-1 {
			b.WriteString(fmt.Sprintf("    %s \\\n", pkg))
		} else {
			b.WriteString(fmt.Sprintf("    %s\n\n", pkg))
		}
	}

	if s.Accounts != nil {
		for _, g := range s.Accounts.Groups {
			b.WriteString(fmt.Sprintf("groupadd -R \"$CHROOT\" --gid %d %s\n", g.GID, g.Name))
		}
		for _, u := range s.Accounts.Users {
			shell := u.Shell
			if shell == "" {
				shell = "/sbin/nologin"
			}
			line := fmt.Sprintf("useradd -R \"$CHROOT\" --uid %d --gid %d -r -m -s %s",
				u.UID, u.GID, shell)
			if len(u.Groups) > 0 {
				line += " -G " + strings.Join(u.Groups, ",")
			}
			line += " " + u.Name
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf(
		"dnf clean all --installroot \"$CHROOT\" --releasever %s\n\n",
		s.Base.Releasever,
	))

	if s.IsImmutable() {
		b.WriteString("# Remove the package manager for true immutability.\n")
		b.WriteString("# The RPM database is retained so 'rpm -qa' works for auditing.\n")
		b.WriteString("rm -rf \\\n")
		b.WriteString("    \"$CHROOT\"/usr/bin/dnf* \\\n")
		b.WriteString("    \"$CHROOT\"/usr/bin/yum* \\\n")
		b.WriteString("    \"$CHROOT\"/usr/lib/python3*/site-packages/dnf \\\n")
		b.WriteString("    \"$CHROOT\"/usr/lib/python3*/site-packages/yum \\\n")
		b.WriteString("    \"$CHROOT/var/cache/dnf\" \\\n")
		b.WriteString("    \"$CHROOT\"/var/log/dnf* \\\n")
		b.WriteString("    \"$CHROOT/tmp/\"*\n")
	}

	return b.String()
}
