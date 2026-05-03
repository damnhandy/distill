package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/damnhandy/distill/internal/spec"
)

func TestArtifactsForPlatform_EmptyPlatformsMatchAll(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{Type: "http", URL: "https://example.com/tool", Dest: "/usr/bin/tool"},
	}
	result := artifactsForPlatform(artifacts, "linux/amd64")
	assert.Len(t, result, 1)
	result2 := artifactsForPlatform(artifacts, "linux/arm64")
	assert.Len(t, result2, 1)
}

func TestArtifactsForPlatform_Filtered(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{Type: "http", URL: "https://example.com/tool-amd64", Dest: "/usr/bin/tool", Platforms: []string{"linux/amd64"}},
		{Type: "http", URL: "https://example.com/tool-arm64", Dest: "/usr/bin/tool", Platforms: []string{"linux/arm64"}},
	}
	amd64 := artifactsForPlatform(artifacts, "linux/amd64")
	assert.Len(t, amd64, 1)
	assert.Contains(t, amd64[0].URL, "amd64")

	arm64 := artifactsForPlatform(artifacts, "linux/arm64")
	assert.Len(t, arm64, 1)
	assert.Contains(t, arm64[0].URL, "arm64")
}

func TestArtifactInstructions_Empty(t *testing.T) {
	assert.Empty(t, artifactInstructions(nil, "linux/amd64"))
	assert.Empty(t, artifactInstructions([]spec.ArtifactSpec{}, "linux/amd64"))
}

func TestArtifactInstructions_HTTPRawBinary(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{
			Type:   "http",
			URL:    "https://example.com/mytool",
			SHA256: "abc123",
			Dest:   "/usr/local/bin/mytool",
			Mode:   "0755",
		},
	}
	out := artifactInstructions(artifacts, "linux/amd64")

	assert.Contains(t, out, "curl -fsSL https://example.com/mytool -o /chroot/usr/local/bin/mytool")
	assert.Contains(t, out, `echo "abc123  /chroot/usr/local/bin/mytool" | sha256sum -c -`)
	assert.Contains(t, out, "chmod 0755 /chroot/usr/local/bin/mytool")
}

func TestArtifactInstructions_HTTPRawBinary_NoSHA256(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{Type: "http", URL: "https://example.com/tool", Dest: "/usr/bin/tool"},
	}
	out := artifactInstructions(artifacts, "linux/amd64")

	assert.NotContains(t, out, "sha256sum")
}

func TestArtifactInstructions_HTTPTarGz(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{
			Type:    "http",
			URL:     "https://example.com/tool.tar.gz",
			SHA256:  "deadbeef",
			Dest:    "/usr/local",
			Extract: "tar.gz",
			Strip:   1,
		},
	}
	out := artifactInstructions(artifacts, "linux/amd64")

	assert.Contains(t, out, "curl -fsSL https://example.com/tool.tar.gz -o /tmp/distill-artifact-0.tar.gz")
	assert.Contains(t, out, `echo "deadbeef  /tmp/distill-artifact-0.tar.gz" | sha256sum -c -`)
	assert.Contains(t, out, "mkdir -p /chroot/usr/local")
	assert.Contains(t, out, "tar -xz --strip-components=1 -f /tmp/distill-artifact-0.tar.gz -C /chroot/usr/local")
	assert.Contains(t, out, "rm /tmp/distill-artifact-0.tar.gz")
}

func TestArtifactInstructions_HTTPZip(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{
			Type:    "http",
			URL:     "https://example.com/tool.zip",
			Dest:    "/usr/local/aws-cli",
			Extract: "zip",
		},
	}
	out := artifactInstructions(artifacts, "linux/amd64")

	assert.Contains(t, out, "curl -fsSL https://example.com/tool.zip -o /tmp/distill-artifact-0.zip")
	assert.Contains(t, out, "unzip -q /tmp/distill-artifact-0.zip -d /chroot/usr/local/aws-cli")
	assert.Contains(t, out, "rm /tmp/distill-artifact-0.zip")
}

func TestArtifactInstructions_Local(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{
			Type: "local",
			Path: "./dist/mytool",
			Dest: "/usr/local/bin/mytool",
			Mode: "0755",
		},
	}
	out := artifactInstructions(artifacts, "linux/amd64")

	assert.Contains(t, out, "COPY distill-artifact-0 /tmp/distill-artifact-0")
	assert.Contains(t, out, "cp /tmp/distill-artifact-0 /chroot/usr/local/bin/mytool")
	assert.Contains(t, out, "chmod 0755 /chroot/usr/local/bin/mytool")
	assert.Contains(t, out, "rm /tmp/distill-artifact-0")
}

func TestArtifactInstructions_PlatformFiltered(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{
			Type:      "http",
			URL:       "https://example.com/tool-amd64",
			Dest:      "/usr/bin/tool",
			Platforms: []string{"linux/amd64"},
		},
		{
			Type:      "http",
			URL:       "https://example.com/tool-arm64",
			Dest:      "/usr/bin/tool",
			Platforms: []string{"linux/arm64"},
		},
	}

	amd64Out := artifactInstructions(artifacts, "linux/amd64")
	assert.Contains(t, amd64Out, "tool-amd64")
	assert.NotContains(t, amd64Out, "tool-arm64")

	arm64Out := artifactInstructions(artifacts, "linux/arm64")
	assert.Contains(t, arm64Out, "tool-arm64")
	assert.NotContains(t, arm64Out, "tool-amd64")
}

func TestNeedsUnzip(t *testing.T) {
	artifacts := []spec.ArtifactSpec{
		{Type: "http", URL: "https://example.com/tool.zip", Dest: "/tmp", Extract: "zip"},
	}
	assert.True(t, needsUnzip(artifacts, "linux/amd64"))
	assert.False(t, needsUnzip([]spec.ArtifactSpec{
		{Type: "http", URL: "https://example.com/tool.tar.gz", Dest: "/tmp", Extract: "tar.gz"},
	}, "linux/amd64"))
}

func TestCopyLocalArtifacts(t *testing.T) {
	// Create a source file in a temp directory.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "mytool")
	require.NoError(t, os.WriteFile(srcFile, []byte("binary content"), 0o755))

	dstDir := t.TempDir()

	artifacts := []spec.ArtifactSpec{
		{Type: "local", Path: "mytool", Dest: "/usr/bin/mytool"},
	}

	err := copyLocalArtifacts(artifacts, "linux/amd64", dstDir, srcDir)
	require.NoError(t, err)

	dstFile := filepath.Join(dstDir, "distill-artifact-0")
	data, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "binary content", string(data))
}

func TestCopyLocalArtifacts_MissingFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	artifacts := []spec.ArtifactSpec{
		{Type: "local", Path: "nonexistent", Dest: "/usr/bin/tool"},
	}

	err := copyLocalArtifacts(artifacts, "linux/amd64", dstDir, srcDir)
	assert.ErrorContains(t, err, "nonexistent")
}

func TestCopyLocalArtifacts_PlatformFiltered(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Only linux/amd64 artifact; building for arm64 should copy nothing.
	artifacts := []spec.ArtifactSpec{
		{Type: "local", Path: "tool", Dest: "/usr/bin/tool", Platforms: []string{"linux/amd64"}},
	}

	err := copyLocalArtifacts(artifacts, "linux/arm64", dstDir, srcDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(dstDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestDNFDockerfile_WithArtifacts(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	s.Contents.Artifacts = []spec.ArtifactSpec{
		{
			Type:   "http",
			URL:    "https://example.com/tool",
			SHA256: "abc",
			Dest:   "/usr/local/bin/tool",
			Mode:   "0755",
		},
	}
	df := dnfDockerfile(s, "linux/amd64")

	assert.Contains(t, df, "curl -fsSL https://example.com/tool")
	assert.Contains(t, df, "/chroot/usr/local/bin/tool")
}

func TestAPTDockerfile_WithZipArtifact_InstallsUnzip(t *testing.T) {
	s := aptSpec(t, []string{"libc6"}, "", nil)
	s.Contents.Artifacts = []spec.ArtifactSpec{
		{Type: "http", URL: "https://example.com/tool.zip", Dest: "/usr/local", Extract: "zip"},
	}
	df := aptDockerfile(s, "linux/amd64")

	assert.Contains(t, df, "apt-get install -y -qq unzip")
}
