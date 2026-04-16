package builder

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Dependency describes a runtime tool that distill shells out to.
type Dependency struct {
	// Name is the executable name looked up on PATH.
	Name string
	// Required indicates the tool must be present for at least one command to work.
	Required bool
	// UsedBy lists the distill sub-commands that invoke this tool.
	UsedBy []string
	// Description is a short human-readable note shown in doctor output.
	Description string
	// MinVersion is the minimum acceptable version (e.g. "2.0.0").
	// Empty means no version requirement is enforced.
	MinVersion string
	// VersionArgs are the arguments passed to the tool to print its version.
	VersionArgs []string
}

// DependencyStatus is the result of checking a single Dependency.
type DependencyStatus struct {
	Dependency
	// Found is true when the executable was located on PATH.
	Found bool
	// Path is the resolved executable path, or empty when not found.
	Path string
	// Version is the parsed version string (e.g. "29.4.0"), or empty when
	// the tool was not found or its version output could not be parsed.
	Version string
	// VersionOK is true when Found is true and either MinVersion is empty or
	// Version >= MinVersion. It is false when the version is unparseable and
	// MinVersion is set.
	VersionOK bool
}

// dependencies is the canonical list of tools distill shells out to.
// The container runtime (docker/podman) is handled separately via DetectCLI.
var dependencies = []Dependency{
	{
		Name:        "podman",
		Required:    false, // required on Linux; docker is the alternative
		UsedBy:      []string{"build"},
		Description: "Container runtime used on Linux and WSL2",
		MinVersion:  "4.0.0",
		VersionArgs: []string{"--version"},
	},
	{
		Name:        "docker",
		Required:    false, // required on macOS/Windows; podman is the alternative
		UsedBy:      []string{"build"},
		Description: "Container runtime used on macOS and Windows",
		MinVersion:  "20.10.0",
		VersionArgs: []string{"--version"},
	},
	{
		Name:        "cosign",
		Required:    true,
		UsedBy:      []string{"provenance", "publish"},
		Description: "Signs and attaches SLSA provenance attestations to OCI images",
		MinVersion:  "2.0.0",
		VersionArgs: []string{"version"},
	},
	{
		Name:        "syft",
		Required:    true,
		UsedBy:      []string{"attest", "publish", "build"},
		Description: "Generates SPDX SBOMs for OCI images",
		MinVersion:  "1.0.0",
		VersionArgs: []string{"--version"},
	},
	{
		Name:        "grype",
		Required:    true,
		UsedBy:      []string{"scan", "publish", "build"},
		Description: "Scans OCI images for known CVEs",
		MinVersion:  "0.70.0",
		VersionArgs: []string{"--version"},
	},
	{
		Name:        "skopeo",
		Required:    false,
		UsedBy:      []string{"provenance"},
		Description: "Resolves base image digests for provenance materials (optional)",
		MinVersion:  "1.0.0",
		VersionArgs: []string{"--version"},
	},
}

// reVersion matches the first X.Y.Z triplet in any string.
var reVersion = regexp.MustCompile(`\d+\.\d+\.\d+`)

// extractVersion returns the first "X.Y.Z" found in output, or empty string.
func extractVersion(output string) string {
	return reVersion.FindString(output)
}

// parseSemver parses a version string of the form "X.Y.Z" or "vX.Y.Z" into
// three integers. Returns [0, 0, 0] if parsing fails.
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return [3]int{}
	}
	return [3]int{major, minor, patch}
}

// versionAtLeast reports whether have >= min. Returns false if either string
// is empty or unparseable.
func versionAtLeast(have, min string) bool {
	if have == "" || min == "" {
		return false
	}
	h := parseSemver(have)
	m := parseSemver(min)
	// Both parsed to [0,0,0] means both were unparseable — treat as unknown.
	if h == ([3]int{}) && m != ([3]int{}) {
		return false
	}
	for i := range m {
		if h[i] != m[i] {
			return h[i] > m[i]
		}
	}
	return true
}

// queryVersion runs the tool with the given args, captures its combined output,
// and returns the first X.Y.Z version string found. Returns empty string on
// any error or if no version pattern is found.
func queryVersion(ctx context.Context, name string, args []string) string {
	var buf bytes.Buffer
	// run() is the package-level trusted subprocess wrapper; G204 is excluded
	// for this package in .golangci.yml.
	_ = run(ctx, &buf, name, args...)
	return extractVersion(buf.String())
}

// CheckAll returns the status of every known runtime dependency, including
// version information obtained by running each tool's version subcommand.
func CheckAll(ctx context.Context) []DependencyStatus {
	results := make([]DependencyStatus, len(dependencies))
	for i, dep := range dependencies {
		status := DependencyStatus{Dependency: dep}
		p, err := exec.LookPath(dep.Name)
		if err == nil {
			status.Found = true
			status.Path = p
			if len(dep.VersionArgs) > 0 {
				status.Version = queryVersion(ctx, dep.Name, dep.VersionArgs)
			}
			if dep.MinVersion == "" {
				status.VersionOK = true
			} else {
				status.VersionOK = versionAtLeast(status.Version, dep.MinVersion)
			}
		}
		results[i] = status
	}
	return results
}

// DetectedPackageManager returns the first package manager found on PATH
// from a priority-ordered list. Devbox and Nix are checked first across all
// platforms because they can be scoped to a project shell, unlike Homebrew,
// dnf, apt-get, and apk which install tools globally.
//
// Priority: devbox > nix > brew (macOS) > dnf / apt-get / apk (Linux)
func DetectedPackageManager() string {
	if _, err := exec.LookPath("devbox"); err == nil {
		return "devbox"
	}
	if _, err := exec.LookPath("nix"); err == nil {
		return "nix"
	}
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			return "brew"
		}
	case "linux":
		for _, pm := range []string{"dnf", "apt-get", "apk"} {
			if _, err := exec.LookPath(pm); err == nil {
				return pm
			}
		}
	}
	return ""
}

// installHints maps tool name → package manager → install command.
// Empty string means the tool is not available via that package manager
// and the user should be directed to the project's GitHub releases.
var installHints = map[string]map[string]string{
	"podman": {
		"brew":    "brew install podman",
		"devbox":  "devbox add podman",
		"nix":     "nix profile install nixpkgs#podman",
		"dnf":     "sudo dnf install -y podman",
		"apt-get": "sudo apt-get install -y podman",
		"apk":     "apk add podman",
	},
	"docker": {
		"brew":    "brew install --cask docker",
		"dnf":     "sudo dnf install -y docker-ce  # requires Docker CE repo",
		"apt-get": "sudo apt-get install -y docker-ce  # requires Docker CE repo",
		// docker is not available via devbox/nix on macOS — use Docker Desktop
	},
	"cosign": {
		"brew":   "brew install cosign",
		"devbox": "devbox add cosign",
		"nix":    "nix profile install nixpkgs#cosign",
	},
	"syft": {
		"brew":   "brew install syft",
		"devbox": "devbox add syft",
		"nix":    "nix profile install nixpkgs#syft",
	},
	"grype": {
		"brew":   "brew install grype",
		"devbox": "devbox add grype",
		"nix":    "nix profile install nixpkgs#grype",
	},
	"skopeo": {
		"brew":    "brew install skopeo",
		"devbox":  "devbox add skopeo",
		"nix":     "nix profile install nixpkgs#skopeo",
		"dnf":     "sudo dnf install -y skopeo",
		"apt-get": "sudo apt-get install -y skopeo",
		"apk":     "apk add skopeo",
	},
}

// MinVersionFor returns the minimum required version string for the named tool,
// or empty string if the tool is unknown or has no version requirement.
func MinVersionFor(tool string) string {
	for _, d := range dependencies {
		if d.Name == tool {
			return d.MinVersion
		}
	}
	return ""
}

// InstallHint returns the best install command for tool given the detected
// package manager. Returns an empty string when no package is available and
// the caller should fall back to the GitHub releases URL.
func InstallHint(tool, pkgManager string) string {
	if hints, ok := installHints[tool]; ok {
		return hints[pkgManager]
	}
	return ""
}

// ContainerRuntimeStatus reports whether the expected container CLI for the
// current OS is present on PATH, its resolved path, installed version, and
// whether the version meets the minimum requirement.
func ContainerRuntimeStatus(ctx context.Context) (cli ContainerCLI, path, version string, versionOK, found bool) {
	cli = DetectCLI()
	p, err := exec.LookPath(string(cli))
	if err != nil {
		return cli, "", "", false, false
	}
	// Find the matching dependency entry for the detected CLI.
	var dep Dependency
	for _, d := range dependencies {
		if d.Name == string(cli) {
			dep = d
			break
		}
	}
	ver := ""
	if len(dep.VersionArgs) > 0 {
		ver = queryVersion(ctx, string(cli), dep.VersionArgs)
	}
	ok := dep.MinVersion == "" || versionAtLeast(ver, dep.MinVersion)
	return cli, p, ver, ok, true
}
