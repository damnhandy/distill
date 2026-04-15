package builder

import (
	"fmt"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// pathsInstructions generates the Dockerfile RUN statements that create the
// filesystem entries declared in s.Paths inside the builder-stage chroot.
// Returns an empty string when s.Paths is empty.
func pathsInstructions(s *spec.ImageSpec) string {
	if len(s.Paths) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n# Create filesystem entries declared in paths.\n")

	for _, p := range s.Paths {
		chrootPath := "/chroot" + p.Path
		switch p.Type {
		case "directory":
			b.WriteString("RUN ")
			fmt.Fprintf(&b, "mkdir -p %s", chrootPath)
			if p.UID != 0 || p.GID != 0 {
				fmt.Fprintf(&b, " \\\n    && chown %d:%d %s", p.UID, p.GID, chrootPath)
			}
			if p.Mode != "" {
				fmt.Fprintf(&b, " \\\n    && chmod %s %s", p.Mode, chrootPath)
			}
			b.WriteString("\n")

		case "file":
			b.WriteString("RUN ")
			// Write content via printf to avoid shell heredoc portability issues.
			// Content is embedded as a single-quoted shell string with newlines escaped.
			escaped := strings.ReplaceAll(p.Content, "'", "'\\''")
			fmt.Fprintf(&b, "printf '%%s' '%s' > %s", escaped, chrootPath)
			if p.UID != 0 || p.GID != 0 {
				fmt.Fprintf(&b, " \\\n    && chown %d:%d %s", p.UID, p.GID, chrootPath)
			}
			if p.Mode != "" {
				fmt.Fprintf(&b, " \\\n    && chmod %s %s", p.Mode, chrootPath)
			}
			b.WriteString("\n")

		case "symlink":
			b.WriteString("RUN ")
			fmt.Fprintf(&b, "ln -sf %s %s", p.Source, chrootPath)
			b.WriteString("\n")
		}
	}

	return b.String()
}
