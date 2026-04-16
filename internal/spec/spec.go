// Package spec defines the ImageSpec type that drives the distill build pipeline.
package spec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImageSpec defines the desired state of a minimal OCI image.
// It is the sole input to the distill build pipeline.
//
// The schema is inspired by Docker Hardened Images (DHI) conventions.
type ImageSpec struct {
	// Name is a human-readable identifier for the image.
	Name string `yaml:"name"`

	// Description is an optional description of the image's purpose.
	Description string `yaml:"description,omitempty"`

	// Variant controls whether the package manager is removed from the final
	// image. "runtime" removes it (default); "dev" retains it.
	Variant string `yaml:"variant,omitempty"`

	// Platforms lists the target build platforms (e.g., linux/amd64, linux/arm64).
	// Defaults to [linux/amd64, linux/arm64] when empty.
	Platforms []string `yaml:"platforms,omitempty"`

	// Source identifies the distribution image used for the chroot bootstrap.
	Source SourceSpec `yaml:"source"`

	// Destination is the OCI image reference to apply to the built image.
	// When omitted, the image is built but not tagged.
	Destination *DestinationSpec `yaml:"destination,omitempty"`

	// Contents declares what should be installed into the image.
	Contents ContentsSpec `yaml:"contents"`

	// Accounts defines non-root users and groups to create inside the image.
	Accounts *AccountsSpec `yaml:"accounts,omitempty"`

	// Environment is a map of environment variables set in the final image.
	// Equivalent to the ENV Dockerfile instruction.
	Environment map[string]string `yaml:"environment,omitempty"`

	// Entrypoint sets the container entrypoint command.
	Entrypoint []string `yaml:"entrypoint,omitempty"`

	// Cmd is the default command when the container is run.
	Cmd []string `yaml:"cmd,omitempty"`

	// WorkDir sets the working directory inside the container.
	WorkDir string `yaml:"work-dir,omitempty"`

	// Annotations are OCI image metadata labels applied to the final image.
	Annotations map[string]string `yaml:"annotations,omitempty"`

	// Volumes declares mount points inside the container.
	// Equivalent to the VOLUME Dockerfile instruction.
	Volumes []string `yaml:"volumes,omitempty"`

	// Ports declares network ports the container listens on.
	// Format: "<port>/<protocol>" (e.g., "8080/tcp"). Equivalent to EXPOSE.
	Ports []string `yaml:"ports,omitempty"`

	// Paths declares filesystem entries (directories, files, symlinks) to
	// create inside the image chroot during the build stage.
	Paths []PathSpec `yaml:"paths,omitempty"`

	// Runtime optionally installs a language runtime sourced directly from
	// upstream rather than from the distro package manager.
	Runtime *RuntimeSpec `yaml:"runtime,omitempty"`
}

// IsRuntime reports whether the package manager should be removed from the
// final image. Returns true when Variant is "runtime" or unset.
func (s *ImageSpec) IsRuntime() bool {
	return s.Variant == "" || s.Variant == "runtime"
}

// EffectivePlatforms returns the build platforms declared in the spec.
// When Platforms is empty it defaults to [linux/amd64, linux/arm64].
func (s *ImageSpec) EffectivePlatforms() []string {
	if len(s.Platforms) == 0 {
		return []string{"linux/amd64", "linux/arm64"}
	}
	return s.Platforms
}

// RunAsUser returns the UserSpec for the container's runtime user.
// It resolves accounts.run-as by name; when run-as is unset it returns the
// first user entry. Returns nil when no users are configured.
func (s *ImageSpec) RunAsUser() *UserSpec {
	if s.Accounts == nil || len(s.Accounts.Users) == 0 {
		return nil
	}
	if s.Accounts.RunAs == "" {
		return &s.Accounts.Users[0]
	}
	for i := range s.Accounts.Users {
		if s.Accounts.Users[i].Name == s.Accounts.RunAs {
			return &s.Accounts.Users[i]
		}
	}
	return nil
}

// ContentsSpec declares what should be installed into the image.
type ContentsSpec struct {
	// Packages is the explicit list of packages to install.
	// Only these packages and their hard dependencies will be present.
	Packages []string `yaml:"packages"`
	// future: Repositories, Keyring for custom package sources
}

// SourceSpec identifies the source distribution for the chroot bootstrap.
type SourceSpec struct {
	// Image is the OCI image reference used as the build host.
	//   registry.access.redhat.com/ubi9/ubi  — RHEL/UBI9
	//   debian:bookworm                       — Debian
	//   ubuntu:24.04                          — Ubuntu
	Image string `yaml:"image"`

	// Releasever is the distribution release version passed to the package
	// manager. For DNF: --releasever value (e.g. "9").
	// For APT/debootstrap: the suite name (e.g. "bookworm", "noble").
	Releasever string `yaml:"releasever"`

	// PackageManager selects the backend. Supported: "dnf", "apt".
	// When omitted, distill infers from the source image reference.
	PackageManager string `yaml:"packageManager,omitempty"`
}

// DestinationSpec defines the OCI image reference for the built output.
type DestinationSpec struct {
	// Image is the registry and image name, without a tag.
	// Example: ghcr.io/damnhandy/rhel9-distilled
	Image string `yaml:"image"`

	// Releasever is the image tag applied to the built image.
	// Defaults to "latest" when omitted.
	Releasever string `yaml:"releasever,omitempty"`
}

// Ref returns the full OCI image reference in the form image:tag.
// When Releasever is empty, "latest" is used as the tag.
func (d DestinationSpec) Ref() string {
	if d.Releasever == "" {
		return d.Image + ":latest"
	}
	return d.Image + ":" + d.Releasever
}

// AccountsSpec defines the non-root users and groups inside the image.
type AccountsSpec struct {
	// RunAs names the user the container process runs as.
	// Must match one of the users entries. Defaults to the first user.
	RunAs  string      `yaml:"run-as,omitempty"`
	Users  []UserSpec  `yaml:"users,omitempty"`
	Groups []GroupSpec `yaml:"groups,omitempty"`
}

// GroupSpec defines a group to create inside the image.
type GroupSpec struct {
	Name    string   `yaml:"name"`
	GID     int      `yaml:"gid"`
	Members []string `yaml:"members,omitempty"`
}

// UserSpec defines a non-root user to create inside the image.
type UserSpec struct {
	Name string `yaml:"name"`
	UID  int    `yaml:"uid"`
	GID  int    `yaml:"gid"`
	// Shell defaults to /sbin/nologin (DNF) or /usr/sbin/nologin (APT).
	Shell string `yaml:"shell,omitempty"`
	// Groups lists additional groups the user should belong to.
	Groups []string `yaml:"groups,omitempty"`
}

// PathSpec declares a filesystem entry to create inside the image chroot.
type PathSpec struct {
	// Type is one of "directory", "file", or "symlink".
	Type string `yaml:"type"`
	// Path is the destination path inside the image (e.g., /app/data).
	Path string `yaml:"path"`
	// Source is the link target — only valid for type: symlink.
	Source string `yaml:"source,omitempty"`
	// Content is the file content — only valid for type: file.
	Content string `yaml:"content,omitempty"`
	UID     int    `yaml:"uid,omitempty"`
	GID     int    `yaml:"gid,omitempty"`
	// Mode is the octal permission string (e.g., "0755").
	Mode string `yaml:"mode,omitempty"`
}

// RuntimeSpec installs a language runtime from an upstream binary distribution
// rather than from the distro package manager.
type RuntimeSpec struct {
	// Type identifies the runtime. Supported: "nodejs", "temurin", "python".
	Type string `yaml:"type"`

	// Version is the exact upstream release to install.
	Version string `yaml:"version"`

	// SHA256 is the expected checksum of the upstream archive.
	SHA256 string `yaml:"sha256"`
}

// Parse unmarshals an ImageSpec from YAML bytes and validates it.
func Parse(data []byte) (*ImageSpec, error) {
	var s ImageSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing image spec: %w", err)
	}
	if err := validate(&s); err != nil {
		return nil, err
	}
	if s.Source.PackageManager == "" {
		s.Source.PackageManager = InferPackageManager(s.Source.Image)
	}
	return &s, nil
}

func validate(s *ImageSpec) error {
	var errs []string
	if s.Name == "" {
		errs = append(errs, "name is required")
	}
	if s.Source.Image == "" {
		errs = append(errs, "source.image is required")
	}
	if s.Source.Releasever == "" {
		errs = append(errs, "source.releasever is required")
	}
	if len(s.Contents.Packages) == 0 {
		errs = append(errs, "at least one package is required under contents.packages")
	}
	if s.Variant != "" && s.Variant != "runtime" && s.Variant != "dev" {
		errs = append(errs, `variant must be "runtime" or "dev"`)
	}
	if s.Destination != nil && s.Destination.Image == "" {
		errs = append(errs, "destination.image is required when destination is set")
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid image spec:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// InferPackageManager guesses the package manager from the source image reference.
// Returns "dnf" for RPM-based images, "apt" for Debian/Ubuntu, and "dnf" as the
// default for unrecognized enterprise images.
func InferPackageManager(image string) string {
	rpmPrefixes := []string{
		"registry.access.redhat.com/ubi",
		"registry.redhat.io/ubi",
		"quay.io/centos",
		"quay.io/fedora",
		"centos:", "fedora:", "rockylinux:", "almalinux:",
		"docker.io/redhat/",
	}
	for _, p := range rpmPrefixes {
		if strings.HasPrefix(image, p) {
			return "dnf"
		}
	}
	aptPrefixes := []string{"debian:", "ubuntu:", "docker.io/library/debian:", "docker.io/library/ubuntu:"}
	for _, p := range aptPrefixes {
		if strings.HasPrefix(image, p) {
			return "apt"
		}
	}
	return "dnf" // default to DNF for unrecognized enterprise images
}
