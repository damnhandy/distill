package builder

import (
	"fmt"
	"sort"
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

	for k, v := range s.Environment {
		fmt.Fprintf(&b, "ENV %s=%q\n", k, v)
	}

	if s.WorkDir != "" {
		fmt.Fprintf(&b, "WORKDIR %s\n", s.WorkDir)
	}

	if u := s.RunAsUser(); u != nil {
		fmt.Fprintf(&b, "USER %d:%d\n", u.UID, u.GID)
	}

	if len(s.Entrypoint) > 0 {
		parts := make([]string, len(s.Entrypoint))
		for i, e := range s.Entrypoint {
			parts[i] = fmt.Sprintf("%q", e)
		}
		fmt.Fprintf(&b, "ENTRYPOINT [%s]\n", strings.Join(parts, ", "))
	}

	if len(s.Cmd) > 0 {
		parts := make([]string, len(s.Cmd))
		for i, c := range s.Cmd {
			parts[i] = fmt.Sprintf("%q", c)
		}
		fmt.Fprintf(&b, "CMD [%s]\n", strings.Join(parts, ", "))
	}

	for _, v := range s.Volumes {
		fmt.Fprintf(&b, "VOLUME %q\n", v)
	}

	for _, p := range s.Ports {
		fmt.Fprintf(&b, "EXPOSE %s\n", p)
	}

	// Always add the image title label. Additional annotations follow in
	// sorted key order for deterministic Dockerfile output.
	fmt.Fprintf(&b, "LABEL org.opencontainers.image.title=%q\n", s.Name)
	keys := make([]string, 0, len(s.Annotations))
	for k := range s.Annotations {
		if k != "org.opencontainers.image.title" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "LABEL %s=%q\n", k, s.Annotations[k])
	}

	return b.String()
}
