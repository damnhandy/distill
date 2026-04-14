package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/damnhandy/distill/internal/spec"
)

func aptSpec(t *testing.T, packages []string, immutable *bool, accounts *spec.AccountsSpec) *spec.ImageSpec {
	t.Helper()
	return &spec.ImageSpec{
		Name: "test-debian-image",
		Base: spec.BaseSpec{
			Image:          "debian:bookworm-slim",
			Releasever:     "bookworm",
			PackageManager: "apt",
		},
		Packages:  packages,
		Immutable: immutable,
		Accounts:  accounts,
	}
}

func TestAPTDockerfile_Structure(t *testing.T) {
	s := aptSpec(t, []string{"libc6"}, nil, nil)
	df := aptDockerfile(s)

	assert.Contains(t, df, "FROM debian:bookworm-slim AS builder")
	assert.Contains(t, df, "FROM scratch")
	assert.Contains(t, df, "COPY --from=builder /chroot /")
}

func TestAPTDockerfile_PackageList(t *testing.T) {
	s := aptSpec(t, []string{"libc6", "ca-certificates", "tzdata"}, nil, nil)
	df := aptDockerfile(s)

	assert.Contains(t, df, "debootstrap")
	assert.Contains(t, df, "--variant=minbase")
	assert.Contains(t, df, "libc6")
	assert.Contains(t, df, "ca-certificates")
	assert.Contains(t, df, "tzdata")
	assert.Contains(t, df, "bookworm")
}

func TestAPTDockerfile_PackagesJoined(t *testing.T) {
	s := aptSpec(t, []string{"libc6", "ca-certificates"}, nil, nil)
	df := aptDockerfile(s)

	// debootstrap --include takes a comma-separated list, not separate args.
	assert.Contains(t, df, "--include=libc6,ca-certificates")
}

func TestAPTDockerfile_Immutable(t *testing.T) {
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
			s := aptSpec(t, []string{"libc6"}, tc.immutable, nil)
			df := aptDockerfile(s)

			if tc.wantRemoved {
				assert.Contains(t, df, "dpkg --purge")
				assert.Contains(t, df, "/chroot/usr/bin/apt*")
				assert.Contains(t, df, "/chroot/usr/bin/dpkg*")
			} else {
				assert.NotContains(t, df, "dpkg --purge")
				assert.NotContains(t, df, "/chroot/usr/bin/apt*")
			}
		})
	}
}

func TestAPTDockerfile_Accounts(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Groups: []spec.GroupSpec{
			{Name: "appuser", GID: 10001},
		},
		Users: []spec.UserSpec{
			{Name: "appuser", UID: 10001, GID: 10001},
		},
	}
	s := aptSpec(t, []string{"libc6"}, nil, accounts)
	df := aptDockerfile(s)

	assert.Contains(t, df, "chroot /chroot groupadd --gid 10001 appuser")
	assert.Contains(t, df, "chroot /chroot useradd --uid 10001 --gid 10001")
	assert.Contains(t, df, "appuser")
}

func TestAPTDockerfile_AccountsDefaultShell(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000},
		},
	}
	s := aptSpec(t, []string{"libc6"}, nil, accounts)
	df := aptDockerfile(s)

	// Debian default is /usr/sbin/nologin (not /sbin/nologin like DNF).
	assert.Contains(t, df, "/usr/sbin/nologin")
}

func TestAPTDockerfile_AccountsExplicitShell(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000, Shell: "/bin/sh"},
		},
	}
	s := aptSpec(t, []string{"libc6"}, nil, accounts)
	df := aptDockerfile(s)

	assert.Contains(t, df, "/bin/sh")
}

func TestAPTDockerfile_AccountsAdditionalGroups(t *testing.T) {
	accounts := &spec.AccountsSpec{
		Users: []spec.UserSpec{
			{Name: "worker", UID: 5000, GID: 5000, Groups: []string{"audio", "video"}},
		},
	}
	s := aptSpec(t, []string{"libc6"}, nil, accounts)
	df := aptDockerfile(s)

	assert.Contains(t, df, "-G audio,video")
}

func TestAPTDockerfile_NoAccounts(t *testing.T) {
	s := aptSpec(t, []string{"libc6"}, nil, nil)
	df := aptDockerfile(s)

	assert.NotContains(t, df, "groupadd")
	assert.NotContains(t, df, "useradd")
}

func TestAPTDockerfile_CacheCleanup(t *testing.T) {
	s := aptSpec(t, []string{"libc6"}, nil, nil)
	df := aptDockerfile(s)

	assert.Contains(t, df, "/chroot/var/cache/apt/archives/*.deb")
	assert.Contains(t, df, "/chroot/var/lib/apt/lists/*")
}

func TestAPTDockerfile_DebootstrapInstall(t *testing.T) {
	s := aptSpec(t, []string{"libc6"}, nil, nil)
	df := aptDockerfile(s)

	assert.Contains(t, df, "apt-get update")
	assert.Contains(t, df, "apt-get install -y")
	assert.Contains(t, df, "debootstrap")
}

func TestAPTDockerfile_ScratchStageMetadata(t *testing.T) {
	s := &spec.ImageSpec{
		Name: "my-debian-image",
		Base: spec.BaseSpec{
			Image:          "debian:bookworm-slim",
			Releasever:     "bookworm",
			PackageManager: "apt",
		},
		Packages: []string{"libc6"},
		Image: spec.ImageConfig{
			Cmd: []string{"/bin/sh"},
			Env: map[string]string{"LANG": "en_US.UTF-8"},
		},
		Accounts: &spec.AccountsSpec{
			Users: []spec.UserSpec{{Name: "app", UID: 1000, GID: 1000}},
		},
	}
	df := aptDockerfile(s)

	assert.Contains(t, df, `CMD ["/bin/sh"]`)
	assert.Contains(t, df, "USER 1000:1000")
	assert.Contains(t, df, `LABEL org.opencontainers.image.title="my-debian-image"`)
}
