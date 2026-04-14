// Package spec defines the ImageSpec type that drives the distill build pipeline.
package spec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImageSpec defines the desired state of a minimal OCI image.
// It is the sole input to the distill build pipeline.
type ImageSpec struct {
	// Name is a human-readable identifier for the image.
	Name string `yaml:"name"`

	// Description is an optional description of the image's purpose.
	Description string `yaml:"description,omitempty"`

	// Base identifies the source distribution used for the chroot bootstrap.
	Base BaseSpec `yaml:"base"`

	// Packages is the explicit list of packages to install into the image.
	// Only these packages and their hard dependencies will be present.
	Packages []string `yaml:"packages"`

	// Runtime optionally installs a language runtime sourced directly from
	// upstream (Temurin, Node.js) rather than from the distro package manager,
	// allowing exact version pinning independent of what the distro ships.
	Runtime *RuntimeSpec `yaml:"runtime,omitempty"`

	// Accounts defines non-root users and groups to create inside the image.
	Accounts *AccountsSpec `yaml:"accounts,omitempty"`

	// Image holds OCI image configuration applied to the final container.
	Image ImageConfig `yaml:"image,omitempty"`

	// Immutable removes the package manager after installation so the image
	// cannot self-modify at runtime. Defaults to true when omitted.
	Immutable *bool `yaml:"immutable,omitempty"`
}

// IsImmutable returns true if the package manager should be removed.
// Defaults to true when the field is not set in the spec.
func (s *ImageSpec) IsImmutable() bool {
	if s.Immutable == nil {
		return true
	}
	return *s.Immutable
}

// BaseSpec identifies the source distribution for the chroot bootstrap.
type BaseSpec struct {
	// Image is the OCI image reference used as the build host.
	// It must have the target distro's package manager available.
	//   registry.access.redhat.com/ubi9/ubi  — RHEL/UBI9
	//   debian:bookworm                       — Debian
	//   ubuntu:24.04                          — Ubuntu
	Image string `yaml:"image"`

	// Releasever is the distribution release version passed to the package
	// manager. For DNF: --releasever value (e.g. "9").
	// For APT/debootstrap: the suite name (e.g. "bookworm", "noble").
	Releasever string `yaml:"releasever"`

	// PackageManager selects the backend. Supported: "dnf", "apt".
	// When omitted, distill infers from the base image reference.
	PackageManager string `yaml:"packageManager,omitempty"`
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

// AccountsSpec defines the non-root users and groups inside the image.
type AccountsSpec struct {
	Groups []GroupSpec `yaml:"groups,omitempty"`
	Users  []UserSpec  `yaml:"users,omitempty"`
}

// GroupSpec defines a group to create inside the image.
type GroupSpec struct {
	Name string `yaml:"name"`
	GID  int    `yaml:"gid"`
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

// ImageConfig holds the OCI image configuration for the final container.
type ImageConfig struct {
	// Cmd is the default command when the container is run.
	Cmd []string `yaml:"cmd,omitempty"`
	// Workdir sets the working directory inside the container.
	Workdir string `yaml:"workdir,omitempty"`
	// Env is a map of environment variables to set in the final image.
	Env map[string]string `yaml:"env,omitempty"`
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
	if s.Base.PackageManager == "" {
		s.Base.PackageManager = inferPackageManager(s.Base.Image)
	}
	return &s, nil
}

func validate(s *ImageSpec) error {
	var errs []string
	if s.Name == "" {
		errs = append(errs, "name is required")
	}
	if s.Base.Image == "" {
		errs = append(errs, "base.image is required")
	}
	if s.Base.Releasever == "" {
		errs = append(errs, "base.releasever is required")
	}
	if len(s.Packages) == 0 {
		errs = append(errs, "at least one package is required")
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid image spec:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// inferPackageManager guesses the package manager from the base image reference.
func inferPackageManager(image string) string {
	rpmPrefixes := []string{
		"registry.access.redhat.com/ubi",
		"registry.redhat.io/ubi",
		"quay.io/centos",
		"quay.io/fedora",
		"centos:", "fedora:", "rockylinux:", "almalinux:",
	}
	for _, p := range rpmPrefixes {
		if strings.HasPrefix(image, p) {
			return "dnf"
		}
	}
	aptPrefixes := []string{"debian:", "ubuntu:"}
	for _, p := range aptPrefixes {
		if strings.HasPrefix(image, p) {
			return "apt"
		}
	}
	return "dnf" // default to DNF for unrecognized enterprise images
}
