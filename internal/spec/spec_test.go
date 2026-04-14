package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// Parse
// ----------------------------------------------------------------------------

func TestParse_Valid(t *testing.T) {
	yaml := `
name: test-image
description: A test image
base:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
packages:
  - glibc
  - ca-certificates
`
	spec, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "test-image", spec.Name)
	assert.Equal(t, "A test image", spec.Description)
	assert.Equal(t, "registry.access.redhat.com/ubi9/ubi", spec.Base.Image)
	assert.Equal(t, "9", spec.Base.Releasever)
	assert.Equal(t, []string{"glibc", "ca-certificates"}, spec.Packages)
	// package manager must be inferred from the UBI prefix → dnf
	assert.Equal(t, "dnf", spec.Base.PackageManager)
}

func TestParse_ExplicitPackageManager(t *testing.T) {
	yaml := `
name: my-image
base:
  image: someinternal.registry/custom-image
  releasever: "8"
  packageManager: dnf
packages:
  - glibc
`
	spec, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "dnf", spec.Base.PackageManager)
}

func TestParse_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing name",
			yaml: `
base:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
packages:
  - glibc
`,
			wantErr: "name is required",
		},
		{
			name: "missing base.image",
			yaml: `
name: test
base:
  releasever: "9"
packages:
  - glibc
`,
			wantErr: "base.image is required",
		},
		{
			name: "missing base.releasever",
			yaml: `
name: test
base:
  image: registry.access.redhat.com/ubi9/ubi
packages:
  - glibc
`,
			wantErr: "base.releasever is required",
		},
		{
			name: "missing packages",
			yaml: `
name: test
base:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
`,
			wantErr: "at least one package is required",
		},
		{
			name: "multiple missing fields",
			yaml: `
base:
  image: registry.access.redhat.com/ubi9/ubi
`,
			wantErr: "invalid image spec",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte(":\tinvalid: yaml: ["))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing image spec")
}

func TestParse_ImmutableDefaultsToTrue(t *testing.T) {
	yaml := `
name: test
base:
  image: ubuntu:24.04
  releasever: noble
packages:
  - libc6
`
	spec, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.True(t, spec.IsImmutable())
}

// ----------------------------------------------------------------------------
// inferPackageManager
// ----------------------------------------------------------------------------

func TestInferPackageManager(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		// DNF — Red Hat / UBI
		{"ubi9 registry.access", "registry.access.redhat.com/ubi9/ubi", "dnf"},
		{"ubi9 registry.redhat.io", "registry.redhat.io/ubi9/ubi-minimal", "dnf"},
		// DNF — CentOS / Fedora
		{"centos stream quay.io", "quay.io/centos/centos:stream9", "dnf"},
		{"fedora quay.io", "quay.io/fedora/fedora:40", "dnf"},
		{"centos short", "centos:stream9", "dnf"},
		{"fedora short", "fedora:40", "dnf"},
		{"rocky linux", "rockylinux:9", "dnf"},
		{"alma linux", "almalinux:9", "dnf"},
		// APT — Debian / Ubuntu
		{"debian bookworm", "debian:bookworm-slim", "apt"},
		{"ubuntu 24.04", "ubuntu:24.04", "apt"},
		// Unknown — falls back to DNF
		{"unknown registry", "someregistry.example.com/custom:latest", "dnf"},
		{"empty string", "", "dnf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inferPackageManager(tc.image)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ----------------------------------------------------------------------------
// IsImmutable
// ----------------------------------------------------------------------------

func TestIsImmutable(t *testing.T) {
	t.Run("nil pointer defaults to true", func(t *testing.T) {
		s := &ImageSpec{}
		assert.True(t, s.IsImmutable())
	})

	t.Run("explicit true", func(t *testing.T) {
		v := true
		s := &ImageSpec{Immutable: &v}
		assert.True(t, s.IsImmutable())
	})

	t.Run("explicit false", func(t *testing.T) {
		v := false
		s := &ImageSpec{Immutable: &v}
		assert.False(t, s.IsImmutable())
	})
}
