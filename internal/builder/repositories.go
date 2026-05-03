package builder

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// dnfRepositoryInstructions generates the Dockerfile RUN block that writes
// custom .repo files and imports GPG keys into the chroot before the main
// dnf install step. Returns an empty string when repos is empty.
func dnfRepositoryInstructions(repos []spec.RepositorySpec) string {
	if len(repos) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n# Add custom package repositories.\n")

	for _, r := range repos {
		gpgcheck := "0"
		gpgkeyLine := ""
		if r.GPGKey != "" {
			gpgcheck = "1"
			gpgkeyLine = "gpgkey=" + r.GPGKey + "\n"
		}
		// Build the .repo file content with actual newlines, then base64-encode
		// it for safe embedding. Using printf with the content as the format
		// string risks misinterpretation of % characters in URLs; embedding
		// literal newlines would split the Dockerfile RUN instruction.
		repoContent := fmt.Sprintf("[%s]\nname=%s\nbaseurl=%s\nenabled=1\ngpgcheck=%s\n%s",
			r.Name, r.Name, r.URL, gpgcheck, gpgkeyLine)
		encoded := base64.StdEncoding.EncodeToString([]byte(repoContent))
		repoFile := fmt.Sprintf("/chroot/etc/yum.repos.d/%s.repo", r.Name)

		b.WriteString("RUN ")
		fmt.Fprintf(&b, "printf '%%s' '%s' | base64 -d > %s", encoded, repoFile)
		if r.GPGKey != "" {
			fmt.Fprintf(&b, " \\\n    && rpm --root /chroot --import %s", r.GPGKey)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// aptRepositoryInstructions generates the Dockerfile RUN blocks that add
// custom APT sources and then install all packages via chroot apt-get.
// This is used when custom repositories are configured; debootstrap has
// already run a minimal bootstrap at this point.
// Returns an empty string when repos is empty.
func aptRepositoryInstructions(repos []spec.RepositorySpec, packages []string, platforms []string) string {
	if len(repos) == 0 {
		return ""
	}

	// Derive the architecture list from the spec platforms when not specified
	// per-repo. linux/amd64 → amd64, linux/arm64 → arm64.
	defaultArchs := make([]string, 0, len(platforms))
	for _, p := range platforms {
		parts := strings.SplitN(p, "/", 2)
		if len(parts) == 2 {
			defaultArchs = append(defaultArchs, parts[1])
		}
	}

	var b strings.Builder
	b.WriteString("\n# Add custom package repositories.\n")

	for _, r := range repos {
		b.WriteString("RUN ")
		first := true

		if r.GPGKey != "" {
			fmt.Fprintf(&b, "curl -fsSL %s | gpg --dearmor > /chroot/etc/apt/trusted.gpg.d/%s.gpg",
				r.GPGKey, r.Name)
			first = false
		}

		// Build sources.list entry.
		archs := r.Arch
		if len(archs) == 0 {
			archs = defaultArchs
		}
		components := r.Components
		if len(components) == 0 {
			components = []string{"main"}
		}

		var bracketParts []string
		if r.GPGKey != "" {
			bracketParts = append(bracketParts, fmt.Sprintf("signed-by=/etc/apt/trusted.gpg.d/%s.gpg", r.Name))
		}
		if len(archs) > 0 {
			bracketParts = append(bracketParts, "arch="+strings.Join(archs, ","))
		}

		var sourceLine string
		if len(bracketParts) > 0 {
			sourceLine = fmt.Sprintf("deb [%s] %s %s %s",
				strings.Join(bracketParts, " "), r.URL, r.Suite, strings.Join(components, " "))
		} else {
			sourceLine = fmt.Sprintf("deb %s %s %s", r.URL, r.Suite, strings.Join(components, " "))
		}

		// Base64-encode the sources.list line for the same reason as repo files:
		// URLs can contain % and the line itself could contain characters that
		// are unsafe as a printf format string.
		encoded := base64.StdEncoding.EncodeToString([]byte(sourceLine + "\n"))
		sourcesFile := fmt.Sprintf("/chroot/etc/apt/sources.list.d/%s.list", r.Name)
		if !first {
			b.WriteString(" \\\n    && ")
		}
		fmt.Fprintf(&b, "printf '%%s' '%s' | base64 -d > %s", encoded, sourcesFile)
		_ = first // suppress unused-after-assignment; kept for readability with the GPG branch

		b.WriteString("\n")
	}

	// Update and install all packages (including those from custom repos).
	b.WriteString("\n# Install packages from all repositories (standard + custom).\n")
	b.WriteString("RUN chroot /chroot apt-get update -qq \\\n")
	b.WriteString("    && chroot /chroot apt-get install -y -q \\\n")
	for i, pkg := range packages {
		if i < len(packages)-1 {
			fmt.Fprintf(&b, "        %s \\\n", pkg)
		} else {
			fmt.Fprintf(&b, "        %s\n", pkg)
		}
	}

	return b.String()
}
