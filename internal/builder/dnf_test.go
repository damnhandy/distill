package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/damnhandy/distill/internal/spec"
)

func boolPtr(b bool) *bool { return &b }

func baseSpec(t *testing.T, packages []string, immutable *bool, accounts *spec.AccountsSpec) *spec.ImageSpec {
	t.Helper()
	return &spec.ImageSpec{
		Name: "test-image",
		Base: spec.BaseSpec{
			Image:          "registry.access.redhat.com/ubi9/ubi",
			Releasever:     "9",
			PackageManager: "dnf",
		},
		Packages:  packages,
		Immutable: immutable,
		Accounts:  accounts,
	}
}

func TestDNFDockerfile_Structure(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, nil, nil)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "FROM registry.access.redhat.com/ubi9/ubi AS builder")
	assert.Contains(t, df, "FROM scratch")
	assert.Contains(t, df, "COPY --from=builder /chroot /")
}

func TestDNFDockerfile_PackageList(t *testing.T) {
	s := baseSpec(t, []string{"glibc", "ca-certificates", "tzdata"}, nil, nil)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "rpm --root /chroot --initdb")
	assert.Contains(t, df, "dnf install -y -q")
	assert.Contains(t, df, "--releasever 9")
	assert.Contains(t, df, "glibc")
	assert.Contains(t, df, "ca-certificates")
	assert.Contains(t, df, "tzdata")
}

func TestDNFDockerfile_SinglePackage(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, nil, nil)
	df := dnfDockerfile(s)

	// The last (and only) package must not have a trailing backslash.
	assert.Contains(t, df, "    glibc\n")
	assert.NotContains(t, df, "    glibc \\\n")
}

func TestDNFDockerfile_MultiplePackagesFormatting(t *testing.T) {
	s := baseSpec(t, []string{"glibc", "ca-certificates"}, nil, nil)
	df := dnfDockerfile(s)

	// All packages except the last get a continuation backslash.
	assert.Contains(t, df, "    glibc \\\n")
	// The final package has no backslash.
	assert.Contains(t, df, "    ca-certificates\n")
}

func TestDNFDockerfile_Immutable(t *testing.T) {
	tests := []struct {
		name        string
		immutable   *bool
		wantRemoved bool
	}{
		{"nil defaults to immutable", nil, true},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseSpec(t, []string{"glibc"}, tc.immutable, nil)
			df := dnfDockerfile(s)

			if tc.wantRemoved {
				assert.Contains(t, df, "/chroot/usr/bin/dnf*")
				assert.Contains(t, df, "/chroot/usr/bin/yum*")
			} else {
				assert.NotContains(t, df, "/chroot/usr/bin/dnf*")
				assert.NotContains(t, df, "/chroot/usr/bin/yum*")
			}
		})
	}
}

func TestDNFDockerfile_Accounts(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Groups: []spec.GroupSpec{
			{Name: "appuser", GID: 10001},
		},
		Users: []spec.UserSpec{
			{Name: "appuser", UID: 10001, GID: 10001},
		},
	}
	s := baseSpec(t, []string{"glibc"}, nil, accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "groupadd -R /chroot --gid 10001 appuser")
	assert.Contains(t, df, "useradd -R /chroot --uid 10001 --gid 10001")
	assert.Contains(t, df, "/sbin/nologin")
	assert.Contains(t, df, "appuser")
}

func TestDNFDockerfile_AccountsDefaultShell(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000},
		},
	}
	s := baseSpec(t, []string{"glibc"}, nil, accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "/sbin/nologin")
}

func TestDNFDockerfile_AccountsExplicitShell(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000, Shell: "/bin/sh"},
		},
	}
	s := baseSpec(t, []string{"glibc"}, nil, accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "/bin/sh")
}

func TestDNFDockerfile_AccountsAdditionalGroups(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000, Groups: []string{"audio", "video"}},
		},
	}
	s := baseSpec(t, []string{"glibc"}, nil, accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "-G audio,video")
}

func TestDNFDockerfile_NoAccounts(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, nil, nil)
	df := dnfDockerfile(s)

	assert.NotContains(t, df, "groupadd")
	assert.NotContains(t, df, "useradd")
}

func TestDNFDockerfile_Cleanup(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, nil, nil)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "dnf clean all --installroot /chroot")
}

func TestDNFDockerfile_ScratchStageMetadata(t *testing.T) {
	s := &spec.ImageSpec{
		Name: "my-image",
		Base: spec.BaseSpec{
			Image:          "registry.access.redhat.com/ubi9/ubi",
			Releasever:     "9",
			PackageManager: "dnf",
		},
		Packages: []string{"glibc"},
		Image: spec.ImageConfig{
			Cmd:     []string{"/bin/bash"},
			Workdir: "/app",
			Env:     map[string]string{"LANG": "en_US.UTF-8"},
		},
		Accounts: &spec.AccountsSpec{
			Users: []spec.UserSpec{{Name: "app", UID: 1000, GID: 1000}},
		},
	}
	df := dnfDockerfile(s)

	assert.Contains(t, df, `CMD ["/bin/bash"]`)
	assert.Contains(t, df, "WORKDIR /app")
	assert.Contains(t, df, "USER 1000:1000")
	assert.Contains(t, df, `LABEL org.opencontainers.image.title="my-image"`)
}
