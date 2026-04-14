package builder

import (
	"fmt"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// scratchStageInstructions generates the final FROM scratch stage of a
// multi-stage Dockerfile from the OCI image configuration in the spec.
// The builder stage is expected to have populated /chroot with the rootfs.
func scratchStageInstructions(s *spec.ImageSpec) string {
	var b strings.Builder

	b.WriteString("\nFROM scratch\n")
	b.WriteString("COPY --from=builder /chroot /\n")

	for k, v := range s.Image.Env {
		b.WriteString(fmt.Sprintf("ENV %s=%q\n", k, v))
	}

	if s.Image.Workdir != "" {
		b.WriteString(fmt.Sprintf("WORKDIR %s\n", s.Image.Workdir))
	}

	if s.Accounts != nil && len(s.Accounts.Users) > 0 {
		u := s.Accounts.Users[0]
		b.WriteString(fmt.Sprintf("USER %d:%d\n", u.UID, u.GID))
	}

	if len(s.Image.Cmd) > 0 {
		parts := make([]string, len(s.Image.Cmd))
		for i, c := range s.Image.Cmd {
			parts[i] = fmt.Sprintf("%q", c)
		}
		b.WriteString(fmt.Sprintf("CMD [%s]\n", strings.Join(parts, ", ")))
	}

	b.WriteString(fmt.Sprintf("LABEL org.opencontainers.image.title=%q\n", s.Name))

	return b.String()
}
