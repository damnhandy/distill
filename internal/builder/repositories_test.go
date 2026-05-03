package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/damnhandy/distill/internal/spec"
)

func TestDNFRepositoryInstructions_Empty(t *testing.T) {
	assert.Empty(t, dnfRepositoryInstructions(nil))
	assert.Empty(t, dnfRepositoryInstructions([]spec.RepositorySpec{}))
}

func TestDNFRepositoryInstructions_NoGPGKey(t *testing.T) {
	repos := []spec.RepositorySpec{
		{Name: "myrepo", URL: "https://example.com/repo/$releasever/$basearch"},
	}
	out := dnfRepositoryInstructions(repos)

	assert.Contains(t, out, "[myrepo]")
	assert.Contains(t, out, "baseurl=https://example.com/repo/$releasever/$basearch")
	assert.Contains(t, out, "gpgcheck=0")
	assert.Contains(t, out, "/chroot/etc/yum.repos.d/myrepo.repo")
	assert.NotContains(t, out, "rpm --root /chroot --import")
}

func TestDNFRepositoryInstructions_WithGPGKey(t *testing.T) {
	repos := []spec.RepositorySpec{
		{
			Name:   "hashicorp",
			URL:    "https://rpm.releases.hashicorp.com/RHEL/$releasever/$basearch/stable",
			GPGKey: "https://rpm.releases.hashicorp.com/gpg",
		},
	}
	out := dnfRepositoryInstructions(repos)

	assert.Contains(t, out, "[hashicorp]")
	assert.Contains(t, out, "gpgcheck=1")
	assert.Contains(t, out, "gpgkey=https://rpm.releases.hashicorp.com/gpg")
	assert.Contains(t, out, "rpm --root /chroot --import https://rpm.releases.hashicorp.com/gpg")
}

func TestDNFRepositoryInstructions_MultipleRepos(t *testing.T) {
	repos := []spec.RepositorySpec{
		{Name: "repo1", URL: "https://example.com/repo1"},
		{Name: "repo2", URL: "https://example.com/repo2"},
	}
	out := dnfRepositoryInstructions(repos)

	assert.Contains(t, out, "/chroot/etc/yum.repos.d/repo1.repo")
	assert.Contains(t, out, "/chroot/etc/yum.repos.d/repo2.repo")
}

func TestAPTRepositoryInstructions_Empty(t *testing.T) {
	assert.Empty(t, aptRepositoryInstructions(nil, nil, nil))
	assert.Empty(t, aptRepositoryInstructions([]spec.RepositorySpec{}, nil, nil))
}

func TestAPTRepositoryInstructions_Basic(t *testing.T) {
	repos := []spec.RepositorySpec{
		{
			Name:  "hashicorp",
			URL:   "https://apt.releases.hashicorp.com",
			Suite: "noble",
		},
	}
	packages := []string{"terraform"}
	out := aptRepositoryInstructions(repos, packages, []string{"linux/amd64"})

	assert.Contains(t, out, "/chroot/etc/apt/sources.list.d/hashicorp.list")
	assert.Contains(t, out, "https://apt.releases.hashicorp.com")
	assert.Contains(t, out, "noble")
	assert.Contains(t, out, "chroot /chroot apt-get update")
	assert.Contains(t, out, "chroot /chroot apt-get install")
	assert.Contains(t, out, "terraform")
}

func TestAPTRepositoryInstructions_WithGPGKey(t *testing.T) {
	repos := []spec.RepositorySpec{
		{
			Name:   "hashicorp",
			URL:    "https://apt.releases.hashicorp.com",
			Suite:  "noble",
			GPGKey: "https://apt.releases.hashicorp.com/gpg",
		},
	}
	out := aptRepositoryInstructions(repos, []string{"curl"}, []string{"linux/amd64"})

	assert.Contains(t, out, "curl -fsSL https://apt.releases.hashicorp.com/gpg")
	assert.Contains(t, out, "/chroot/etc/apt/trusted.gpg.d/hashicorp.gpg")
	assert.Contains(t, out, "signed-by=/etc/apt/trusted.gpg.d/hashicorp.gpg")
}

func TestAPTRepositoryInstructions_DefaultComponents(t *testing.T) {
	repos := []spec.RepositorySpec{
		{Name: "myrepo", URL: "https://example.com", Suite: "stable"},
	}
	out := aptRepositoryInstructions(repos, []string{"mypkg"}, []string{"linux/amd64"})

	assert.Contains(t, out, "main")
}

func TestAPTRepositoryInstructions_ExplicitComponents(t *testing.T) {
	repos := []spec.RepositorySpec{
		{Name: "myrepo", URL: "https://example.com", Suite: "stable", Components: []string{"contrib", "non-free"}},
	}
	out := aptRepositoryInstructions(repos, []string{"mypkg"}, []string{"linux/amd64"})

	assert.Contains(t, out, "contrib non-free")
}

func TestDNFDockerfile_WithRepositories(t *testing.T) {
	s := baseSpec(t, []string{"terraform"}, "", nil)
	s.Contents.Repositories = []spec.RepositorySpec{
		{
			Name:   "hashicorp",
			URL:    "https://rpm.releases.hashicorp.com/RHEL/$releasever/$basearch/stable",
			GPGKey: "https://rpm.releases.hashicorp.com/gpg",
		},
	}
	df := dnfDockerfile(s, "linux/amd64")

	// Repo file must appear AFTER rpm init but BEFORE dnf install.
	repoIdx := indexOfStr(df, "yum.repos.d/hashicorp.repo")
	installIdx := indexOfStr(df, "dnf install -y -q")
	assert.Positive(t, repoIdx)
	assert.Greater(t, installIdx, repoIdx, "repo must be added before dnf install")
}

func TestAPTDockerfile_WithRepositories(t *testing.T) {
	s := aptSpec(t, []string{"libc6", "terraform"}, "", nil)
	s.Contents.Repositories = []spec.RepositorySpec{
		{
			Name:   "hashicorp",
			URL:    "https://apt.releases.hashicorp.com",
			Suite:  "bookworm",
			GPGKey: "https://apt.releases.hashicorp.com/gpg",
		},
	}
	df := aptDockerfile(s, "linux/amd64")

	// Two-phase path: debootstrap must NOT have --include when repos are set.
	assert.NotContains(t, df, "--include=libc6")
	assert.Contains(t, df, "debootstrap --foreign --variant=minbase")
	assert.Contains(t, df, "chroot /chroot apt-get install")
	assert.Contains(t, df, "hashicorp.list")
}

// indexOfStr returns the byte offset of substr in s, or -1.
func indexOfStr(s, substr string) int {
	idx := 0
	for i := range s {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return idx
		}
		idx = i + 1
	}
	return -1
}
