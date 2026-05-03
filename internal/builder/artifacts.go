package builder

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/damnhandy/distill/internal/spec"
)

// artifactsForPlatform returns the subset of artifacts that apply to platform.
// An artifact with an empty Platforms list applies to all platforms.
func artifactsForPlatform(artifacts []spec.ArtifactSpec, platform string) []spec.ArtifactSpec {
	var out []spec.ArtifactSpec
	for _, a := range artifacts {
		if len(a.Platforms) == 0 {
			out = append(out, a)
			continue
		}
		for _, p := range a.Platforms {
			if p == platform {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// needsUnzip reports whether any platform-applicable artifact requires unzip.
func needsUnzip(artifacts []spec.ArtifactSpec, platform string) bool {
	for _, a := range artifactsForPlatform(artifacts, platform) {
		if a.Extract == "zip" {
			return true
		}
	}
	return false
}

// artifactInstructions generates the Dockerfile instructions that install
// artifacts into the chroot for the given platform. The index i is used
// as a stable temporary-file suffix to avoid name collisions.
// Returns an empty string when no artifacts apply to platform.
func artifactInstructions(artifacts []spec.ArtifactSpec, platform string) string {
	applicable := artifactsForPlatform(artifacts, platform)
	if len(applicable) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n# Install artifacts.\n")

	for i, a := range applicable {
		switch a.Type {
		case "http":
			writeHTTPArtifact(&b, a, i)
		case "local":
			writeLocalArtifact(&b, a, i)
		}
	}

	return b.String()
}

func writeHTTPArtifact(b *strings.Builder, a spec.ArtifactSpec, i int) {
	chrootDest := "/chroot" + a.Dest

	if a.Extract == "" {
		// Raw binary download.
		b.WriteString("RUN ")
		fmt.Fprintf(b, "curl -fsSL %s -o %s", a.URL, chrootDest)
		if a.SHA256 != "" {
			fmt.Fprintf(b, " \\\n    && echo \"%s  %s\" | sha256sum -c -", a.SHA256, chrootDest)
		}
		if a.Mode != "" {
			fmt.Fprintf(b, " \\\n    && chmod %s %s", a.Mode, chrootDest)
		}
		b.WriteString("\n")
		return
	}

	// Archive download: fetch to temp, verify, extract, clean up.
	tmpFile := fmt.Sprintf("/tmp/distill-artifact-%d.%s", i, a.Extract)
	b.WriteString("RUN ")
	fmt.Fprintf(b, "curl -fsSL %s -o %s", a.URL, tmpFile)
	if a.SHA256 != "" {
		fmt.Fprintf(b, " \\\n    && echo \"%s  %s\" | sha256sum -c -", a.SHA256, tmpFile)
	}
	fmt.Fprintf(b, " \\\n    && mkdir -p %s", chrootDest)

	switch a.Extract {
	case "tar.gz", "tar.bz2", "tar.xz":
		flag := map[string]string{"tar.gz": "z", "tar.bz2": "j", "tar.xz": "J"}[a.Extract]
		if a.Strip > 0 {
			fmt.Fprintf(b, " \\\n    && tar -x%s --strip-components=%d -f %s -C %s",
				flag, a.Strip, tmpFile, chrootDest)
		} else {
			fmt.Fprintf(b, " \\\n    && tar -x%s -f %s -C %s", flag, tmpFile, chrootDest)
		}
	case "zip":
		fmt.Fprintf(b, " \\\n    && unzip -q %s -d %s", tmpFile, chrootDest)
	}

	fmt.Fprintf(b, " \\\n    && rm %s\n", tmpFile)
}

func writeLocalArtifact(b *strings.Builder, a spec.ArtifactSpec, i int) {
	// The file is staged into the build context as distill-artifact-<i>
	// by copyLocalArtifacts before this Dockerfile is written.
	stageName := fmt.Sprintf("distill-artifact-%d", i)
	chrootDest := "/chroot" + a.Dest
	tmpPath := "/tmp/" + stageName

	fmt.Fprintf(b, "COPY %s %s\n", stageName, tmpPath)
	b.WriteString("RUN ")
	fmt.Fprintf(b, "mkdir -p $(dirname %s)", chrootDest)
	fmt.Fprintf(b, " \\\n    && cp %s %s", tmpPath, chrootDest)
	if a.Mode != "" {
		fmt.Fprintf(b, " \\\n    && chmod %s %s", a.Mode, chrootDest)
	}
	fmt.Fprintf(b, " \\\n    && rm %s\n", tmpPath)
}

// copyLocalArtifacts copies local artifact files into contextDir so they can
// be referenced by COPY instructions in the generated Dockerfile.
// sourceDir is the directory containing the spec file; artifact paths are
// resolved relative to it. Returns an error if any source file is missing.
func copyLocalArtifacts(artifacts []spec.ArtifactSpec, platform, contextDir, sourceDir string) error {
	applicable := artifactsForPlatform(artifacts, platform)
	for i, a := range applicable {
		if a.Type != "local" {
			continue
		}
		src := filepath.Join(sourceDir, a.Path)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("artifact %q: %w", a.Path, err)
		}
		dst := filepath.Join(contextDir, fmt.Sprintf("distill-artifact-%d", i))
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("staging artifact %q: %w", a.Path, err)
		}
	}
	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src) //nolint:gosec // path is resolved from user-provided spec
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode()) //nolint:gosec // dst is within the temp build context dir
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}
