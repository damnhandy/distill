package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/damnhandy/distill/internal/spec"
)

func baseSpec(t *testing.T, packages []string, variant string, accounts *spec.AccountsSpec) *spec.ImageSpec {
	t.Helper()
	return &spec.ImageSpec{
		Name: "test-image",
		Source: spec.SourceSpec{
			Image:          "registry.access.redhat.com/ubi9/ubi",
			Releasever:     "9",
			PackageManager: "dnf",
		},
		Contents: spec.ContentsSpec{Packages: packages},
		Variant:  variant,
		Accounts: accounts,
	}
}

func TestDNFDockerfile_Structure(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "FROM registry.access.redhat.com/ubi9/ubi AS builder")
	assert.Contains(t, df, "FROM scratch")
	assert.Contains(t, df, "COPY --from=builder /chroot /")
}

func TestDNFDockerfile_PackageList(t *testing.T) {
	s := baseSpec(t, []string{"glibc", "ca-certificates", "tzdata"}, "", nil)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "rpm --root /chroot --initdb")
	assert.Contains(t, df, "dnf install -y -q")
	assert.Contains(t, df, "--releasever 9")
	assert.Contains(t, df, "glibc")
	assert.Contains(t, df, "ca-certificates")
	assert.Contains(t, df, "tzdata")
}

func TestDNFDockerfile_SinglePackage(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	df := dnfDockerfile(s)

	// The last (and only) package must not have a trailing backslash.
	assert.Contains(t, df, "    glibc\n")
	assert.NotContains(t, df, "    glibc \\\n")
}

func TestDNFDockerfile_MultiplePackagesFormatting(t *testing.T) {
	s := baseSpec(t, []string{"glibc", "ca-certificates"}, "", nil)
	df := dnfDockerfile(s)

	// All packages except the last get a continuation backslash.
	assert.Contains(t, df, "    glibc \\\n")
	// The final package has no backslash.
	assert.Contains(t, df, "    ca-certificates\n")
}

func TestDNFDockerfile_RuntimeVariant(t *testing.T) {
	tests := []struct {
		name        string
		variant     string
		wantRemoved bool
	}{
		{"empty defaults to runtime", "", true},
		{"explicit runtime", "runtime", true},
		{"dev variant keeps package manager", "dev", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseSpec(t, []string{"glibc"}, tc.variant, nil)
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
	s := baseSpec(t, []string{"glibc"}, "", accounts)
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
	s := baseSpec(t, []string{"glibc"}, "", accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "/sbin/nologin")
}

func TestDNFDockerfile_AccountsExplicitShell(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000, Shell: "/bin/sh"},
		},
	}
	s := baseSpec(t, []string{"glibc"}, "", accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "/bin/sh")
}

func TestDNFDockerfile_AccountsAdditionalGroups(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000, Groups: []string{"audio", "video"}},
		},
	}
	s := baseSpec(t, []string{"glibc"}, "", accounts)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "-G audio,video")
}

func TestDNFDockerfile_NoAccounts(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	df := dnfDockerfile(s)

	assert.NotContains(t, df, "groupadd")
	assert.NotContains(t, df, "useradd")
}

func TestDNFDockerfile_Cleanup(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	df := dnfDockerfile(s)

	assert.Contains(t, df, "dnf clean all --installroot /chroot")
}

func TestDNFDockerfile_ScratchStageMetadata(t *testing.T) {
	s := &spec.ImageSpec{
		Name: "my-image",
		Source: spec.SourceSpec{
			Image:          "registry.access.redhat.com/ubi9/ubi",
			Releasever:     "9",
			PackageManager: "dnf",
		},
		Contents:    spec.ContentsSpec{Packages: []string{"glibc"}},
		Cmd:         []string{"/bin/bash"},
		WorkDir:     "/app",
		Environment: map[string]string{"LANG": "en_US.UTF-8"},
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

func TestDNFDockerfile_RunAs(t *testing.T) {
	s := &spec.ImageSpec{
		Name: "my-image",
		Source: spec.SourceSpec{
			Image:          "registry.access.redhat.com/ubi9/ubi",
			Releasever:     "9",
			PackageManager: "dnf",
		},
		Contents: spec.ContentsSpec{Packages: []string{"glibc"}},
		Accounts: &spec.AccountsSpec{
			RunAs: "worker",
			Users: []spec.UserSpec{
				{Name: "app", UID: 1000, GID: 1000},
				{Name: "worker", UID: 2000, GID: 2000},
			},
		},
	}
	df := dnfDockerfile(s)

	assert.Contains(t, df, "USER 2000:2000")
	assert.NotContains(t, df, "USER 1000:1000")
}

func TestDNFDockerfile_EntrypointAndCmd(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	s.Entrypoint = []string{"/docker-entrypoint.sh"}
	s.Cmd = []string{"nginx", "-g", "daemon off;"}
	df := dnfDockerfile(s)

	assert.Contains(t, df, `ENTRYPOINT ["/docker-entrypoint.sh"]`)
	assert.Contains(t, df, `CMD ["nginx", "-g", "daemon off;"]`)
}

func TestDNFDockerfile_VolumesAndPorts(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	s.Volumes = []string{"/data", "/logs"}
	s.Ports = []string{"8080/tcp", "9090/tcp"}
	df := dnfDockerfile(s)

	assert.Contains(t, df, `VOLUME "/data"`)
	assert.Contains(t, df, `VOLUME "/logs"`)
	assert.Contains(t, df, "EXPOSE 8080/tcp")
	assert.Contains(t, df, "EXPOSE 9090/tcp")
}

func TestDNFDockerfile_Annotations(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	s.Annotations = map[string]string{
		"org.opencontainers.image.source": "https://github.com/example/repo",
	}
	df := dnfDockerfile(s)

	assert.Contains(t, df, `LABEL org.opencontainers.image.title="test-image"`)
	assert.Contains(t, df, `LABEL org.opencontainers.image.source="https://github.com/example/repo"`)
}

func TestDNFDockerfile_Paths(t *testing.T) {
	s := baseSpec(t, []string{"glibc"}, "", nil)
	s.Paths = []spec.PathSpec{
		{Type: "directory", Path: "/app/data", UID: 1000, GID: 1000, Mode: "0755"},
		{Type: "symlink", Path: "/usr/local/bin/app", Source: "/opt/app/bin/app"},
	}
	df := dnfDockerfile(s)

	assert.Contains(t, df, "mkdir -p /chroot/app/data")
	assert.Contains(t, df, "chown 1000:1000 /chroot/app/data")
	assert.Contains(t, df, "chmod 0755 /chroot/app/data")
	assert.Contains(t, df, "ln -sf /opt/app/bin/app /chroot/usr/local/bin/app")
}
