package builder

import (
	"encoding/base64"
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
			// Base64-encode the content so it survives Dockerfile parsing intact.
			// Plain embedding would let newlines split the RUN instruction into
			// separate lines, causing the Dockerfile parser to misread TOML section
			// headers (e.g. [aws]) as unknown instructions.
			encoded := base64.StdEncoding.EncodeToString([]byte(p.Content))
			fmt.Fprintf(&b, "printf '%%s' '%s' | base64 -d > %s", encoded, chrootPath)
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
