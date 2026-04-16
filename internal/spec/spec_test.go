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
	yml := `
name: test-image
description: A test image
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
    - ca-certificates
`
	spec, err := Parse([]byte(yml))
	require.NoError(t, err)
	assert.Equal(t, "test-image", spec.Name)
	assert.Equal(t, "A test image", spec.Description)
	assert.Equal(t, "registry.access.redhat.com/ubi9/ubi", spec.Source.Image)
	assert.Equal(t, "9", spec.Source.Releasever)
	assert.Equal(t, []string{"glibc", "ca-certificates"}, spec.Contents.Packages)
	// package manager must be inferred from the UBI prefix → dnf
	assert.Equal(t, "dnf", spec.Source.PackageManager)
}

func TestParse_ExplicitPackageManager(t *testing.T) {
	yml := `
name: my-image
source:
  image: someinternal.registry/custom-image
  releasever: "8"
  packageManager: dnf
contents:
  packages:
    - glibc
`
	spec, err := Parse([]byte(yml))
	require.NoError(t, err)
	assert.Equal(t, "dnf", spec.Source.PackageManager)
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
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
`,
			wantErr: "name is required",
		},
		{
			name: "missing source.image",
			yaml: `
name: test
source:
  releasever: "9"
contents:
  packages:
    - glibc
`,
			wantErr: "source.image is required",
		},
		{
			name: "missing source.releasever",
			yaml: `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
contents:
  packages:
    - glibc
`,
			wantErr: "source.releasever is required",
		},
		{
			name: "missing packages",
			yaml: `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
`,
			wantErr: "at least one package is required",
		},
		{
			name: "multiple missing fields",
			yaml: `
source:
  image: registry.access.redhat.com/ubi9/ubi
`,
			wantErr: "invalid image spec",
		},
		{
			name: "invalid variant",
			yaml: `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
variant: immutable
`,
			wantErr: `variant must be "runtime" or "dev"`,
		},
		{
			name: "destination with empty image",
			yaml: `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
destination:
  releasever: latest
`,
			wantErr: "destination.image is required",
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

// ----------------------------------------------------------------------------
// DestinationSpec.Ref
// ----------------------------------------------------------------------------

func TestDestinationSpec_Ref(t *testing.T) {
	t.Run("explicit releasever", func(t *testing.T) {
		d := DestinationSpec{Image: "ghcr.io/example/myapp", Releasever: "1.2.3"}
		assert.Equal(t, "ghcr.io/example/myapp:1.2.3", d.Ref())
	})

	t.Run("empty releasever defaults to latest", func(t *testing.T) {
		d := DestinationSpec{Image: "ghcr.io/example/myapp"}
		assert.Equal(t, "ghcr.io/example/myapp:latest", d.Ref())
	})
}

// ----------------------------------------------------------------------------
// IsRuntime
// ----------------------------------------------------------------------------

func TestIsRuntime(t *testing.T) {
	t.Run("unset defaults to runtime", func(t *testing.T) {
		s := &ImageSpec{}
		assert.True(t, s.IsRuntime())
	})

	t.Run("explicit runtime", func(t *testing.T) {
		s := &ImageSpec{Variant: "runtime"}
		assert.True(t, s.IsRuntime())
	})

	t.Run("dev variant", func(t *testing.T) {
		s := &ImageSpec{Variant: "dev"}
		assert.False(t, s.IsRuntime())
	})
}

// ----------------------------------------------------------------------------
// EffectivePlatforms
// ----------------------------------------------------------------------------

func TestEffectivePlatforms(t *testing.T) {
	t.Run("empty defaults to amd64 and arm64", func(t *testing.T) {
		s := &ImageSpec{}
		assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, s.EffectivePlatforms())
	})

	t.Run("explicit platforms returned as-is", func(t *testing.T) {
		s := &ImageSpec{Platforms: []string{"linux/amd64"}}
		assert.Equal(t, []string{"linux/amd64"}, s.EffectivePlatforms())
	})
}

// ----------------------------------------------------------------------------
// RunAsUser
// ----------------------------------------------------------------------------

func TestRunAsUser(t *testing.T) {
	t.Run("nil accounts returns nil", func(t *testing.T) {
		s := &ImageSpec{}
		assert.Nil(t, s.RunAsUser())
	})

	t.Run("no run-as falls back to first user", func(t *testing.T) {
		s := &ImageSpec{
			Accounts: &AccountsSpec{
				Users: []UserSpec{
					{Name: "first", UID: 1000, GID: 1000},
					{Name: "second", UID: 2000, GID: 2000},
				},
			},
		}
		u := s.RunAsUser()
		require.NotNil(t, u)
		assert.Equal(t, "first", u.Name)
	})

	t.Run("run-as resolves by name", func(t *testing.T) {
		s := &ImageSpec{
			Accounts: &AccountsSpec{
				RunAs: "second",
				Users: []UserSpec{
					{Name: "first", UID: 1000, GID: 1000},
					{Name: "second", UID: 2000, GID: 2000},
				},
			},
		}
		u := s.RunAsUser()
		require.NotNil(t, u)
		assert.Equal(t, "second", u.Name)
		assert.Equal(t, 2000, u.UID)
	})

	t.Run("run-as not found returns nil", func(t *testing.T) {
		s := &ImageSpec{
			Accounts: &AccountsSpec{
				RunAs: "nobody",
				Users: []UserSpec{
					{Name: "appuser", UID: 1000, GID: 1000},
				},
			},
		}
		assert.Nil(t, s.RunAsUser())
	})
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
		{"docker.io redhat", "docker.io/redhat/ubi9", "dnf"},
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
			got := InferPackageManager(tc.image)
			assert.Equal(t, tc.want, got)
		})
	}
}
