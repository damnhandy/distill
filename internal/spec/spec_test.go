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
// Pipeline parsing and normalization
// ----------------------------------------------------------------------------

func TestParse_Pipeline(t *testing.T) {
	t.Run("pipeline nil when omitted", func(t *testing.T) {
		yml := `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
`
		s, err := Parse([]byte(yml))
		require.NoError(t, err)
		assert.Nil(t, s.Pipeline)
	})

	t.Run("pipeline defaults applied", func(t *testing.T) {
		yml := `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
pipeline:
  scan:
    enabled: true
  sbom:
    enabled: true
  provenance:
    enabled: true
`
		s, err := Parse([]byte(yml))
		require.NoError(t, err)
		require.NotNil(t, s.Pipeline)
		require.NotNil(t, s.Pipeline.Scan)
		assert.True(t, s.Pipeline.Scan.Enabled)
		assert.Equal(t, "critical", s.Pipeline.Scan.FailOn)
		require.NotNil(t, s.Pipeline.SBOM)
		assert.True(t, s.Pipeline.SBOM.Enabled)
		assert.Equal(t, "sbom.spdx.json", s.Pipeline.SBOM.Output)
		require.NotNil(t, s.Pipeline.Provenance)
		assert.True(t, s.Pipeline.Provenance.Enabled)
	})

	t.Run("explicit pipeline values not overridden by defaults", func(t *testing.T) {
		yml := `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
pipeline:
  scan:
    enabled: true
    fail-on: high
  sbom:
    enabled: true
    output: custom-sbom.spdx.json
`
		s, err := Parse([]byte(yml))
		require.NoError(t, err)
		assert.Equal(t, "high", s.Pipeline.Scan.FailOn)
		assert.Equal(t, "custom-sbom.spdx.json", s.Pipeline.SBOM.Output)
	})

	t.Run("disabled steps with nil sub-specs", func(t *testing.T) {
		yml := `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
pipeline:
  scan:
    enabled: true
`
		s, err := Parse([]byte(yml))
		require.NoError(t, err)
		require.NotNil(t, s.Pipeline)
		require.NotNil(t, s.Pipeline.Scan)
		assert.Nil(t, s.Pipeline.SBOM)
		assert.Nil(t, s.Pipeline.Provenance)
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
// Repositories and Artifacts validation
// ----------------------------------------------------------------------------

func TestParse_Repositories_Valid(t *testing.T) {
	yml := `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - terraform
  repositories:
    - name: hashicorp
      url: https://rpm.releases.hashicorp.com/RHEL/$releasever/$basearch/stable
      gpg-key: https://rpm.releases.hashicorp.com/gpg
`
	s, err := Parse([]byte(yml))
	require.NoError(t, err)
	require.Len(t, s.Contents.Repositories, 1)
	assert.Equal(t, "hashicorp", s.Contents.Repositories[0].Name)
	assert.Equal(t, "https://rpm.releases.hashicorp.com/RHEL/$releasever/$basearch/stable", s.Contents.Repositories[0].URL)
	assert.Equal(t, "https://rpm.releases.hashicorp.com/gpg", s.Contents.Repositories[0].GPGKey)
}

func TestParse_Repositories_Validation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing repo name",
			yaml: `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
  repositories:
    - url: https://example.com/repo
`,
			wantErr: "contents.repositories[0].name is required",
		},
		{
			name: "missing repo url",
			yaml: `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
  repositories:
    - name: myrepo
`,
			wantErr: "contents.repositories[0].url is required",
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

func TestParse_Artifacts_Valid(t *testing.T) {
	yml := `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
  artifacts:
    - type: http
      url: https://example.com/tool
      sha256: abc123
      dest: /usr/local/bin/tool
      mode: "0755"
    - type: local
      path: ./dist/mytool
      dest: /usr/local/bin/mytool
      platforms: [linux/amd64]
`
	s, err := Parse([]byte(yml))
	require.NoError(t, err)
	require.Len(t, s.Contents.Artifacts, 2)
	assert.Equal(t, "http", s.Contents.Artifacts[0].Type)
	assert.Equal(t, "https://example.com/tool", s.Contents.Artifacts[0].URL)
	assert.Equal(t, "abc123", s.Contents.Artifacts[0].SHA256)
	assert.Equal(t, "/usr/local/bin/tool", s.Contents.Artifacts[0].Dest)
	assert.Equal(t, "local", s.Contents.Artifacts[1].Type)
	assert.Equal(t, []string{"linux/amd64"}, s.Contents.Artifacts[1].Platforms)
}

func TestParse_Artifacts_Validation(t *testing.T) {
	baseYAML := func(artifacts string) string {
		return `
name: test
source:
  image: registry.access.redhat.com/ubi9/ubi
  releasever: "9"
contents:
  packages:
    - glibc
  artifacts:
` + artifacts
	}
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "missing type",
			yaml:    baseYAML("    - url: https://example.com/tool\n      dest: /usr/bin/tool\n"),
			wantErr: "contents.artifacts[0].type is required",
		},
		{
			name:    "invalid type",
			yaml:    baseYAML("    - type: ftp\n      dest: /usr/bin/tool\n"),
			wantErr: `contents.artifacts[0].type "ftp" is invalid`,
		},
		{
			name:    "http missing url",
			yaml:    baseYAML("    - type: http\n      dest: /usr/bin/tool\n"),
			wantErr: "contents.artifacts[0].url is required for type http",
		},
		{
			name:    "local missing path",
			yaml:    baseYAML("    - type: local\n      dest: /usr/bin/tool\n"),
			wantErr: "contents.artifacts[0].path is required for type local",
		},
		{
			name:    "missing dest",
			yaml:    baseYAML("    - type: http\n      url: https://example.com/tool\n"),
			wantErr: "contents.artifacts[0].dest is required",
		},
		{
			name:    "invalid extract",
			yaml:    baseYAML("    - type: http\n      url: https://example.com/t\n      dest: /tmp\n      extract: rar\n"),
			wantErr: `contents.artifacts[0].extract "rar" is invalid`,
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
