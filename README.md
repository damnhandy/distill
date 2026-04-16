# distill

Build minimal, immutable OCI images from enterprise Linux base distributions.

distill is a CLI tool that takes a declarative `.distill.yaml` spec and produces a minimal
`FROM scratch` OCI image using a chroot bootstrap strategy. It is a
distro-agnostic alternative to Google's [distroless](https://github.com/GoogleContainerTools/distroless)
images for teams that need images rooted in RHEL, UBI, Debian, or Ubuntu.

## How it works

1. Reads a declarative `.distill.yaml` spec describing the desired packages and configuration
2. Runs a multi-stage Docker/Podman build using the target base image — so the correct package manager, repos, and release version are always available
3. Installs only the listed packages into an isolated chroot directory inside the builder stage
4. Copies the chroot into a `FROM scratch` final stage
5. Commits the result as a minimal OCI image

The package manager is never present in the final image — not removed as a layer, but never copied in to begin with. This is the same reason Chainguard built [`apko`](https://github.com/chainguard-dev/apko) rather than using Dockerfiles; distill is the equivalent for RPM and APT-based enterprise distributions.

## Supported distributions

| Distribution | Package Manager | Example `base.image` |
|---|---|---|
| RHEL / UBI 9 | DNF | `registry.access.redhat.com/ubi9/ubi` |
| CentOS Stream 9 | DNF | `quay.io/centos/centos:stream9` |
| Rocky Linux 9 | DNF | `rockylinux:9` |
| AlmaLinux 9 | DNF | `almalinux:9` |
| Debian Bookworm | APT | `debian:bookworm-slim` |
| Ubuntu 24.04 | APT | `ubuntu:24.04` |

## Installation

### Homebrew (macOS and Linux)

```bash
brew tap damnhandy/tap
brew install damnhandy/tap/distill
```

### Shell installer (Linux, macOS, Alpine, FreeBSD)

```bash
# Install latest to /usr/local/bin (may require sudo)
curl -sfL https://raw.githubusercontent.com/damnhandy/distill/main/scripts/install.sh | sudo sh

# Install to a directory you own (no sudo)
curl -sfL https://raw.githubusercontent.com/damnhandy/distill/main/scripts/install.sh | sh -s -- -b ~/.local/bin

# Install a specific version
curl -sfL https://raw.githubusercontent.com/damnhandy/distill/main/scripts/install.sh | sh -s -- v1.2.3
```

### RPM (RHEL, Fedora, CentOS Stream, Rocky Linux, AlmaLinux)

Download the `.rpm` from the [latest release](https://github.com/damnhandy/distill/releases/latest) and install:

```bash
sudo dnf localinstall distill_<version>_linux_amd64.rpm
```

### DEB (Debian, Ubuntu)

Download the `.deb` from the [latest release](https://github.com/damnhandy/distill/releases/latest) and install:

```bash
sudo dpkg -i distill_<version>_linux_amd64.deb
```

### Nix / NixOS

```bash
# Install to your profile
nix profile install github:damnhandy/distill

# Pin to a specific version
nix profile install github:damnhandy/distill/v1.2.3

# Run without installing
nix run github:damnhandy/distill -- --help
```

For NixOS, add to your `configuration.nix`:

```nix
{ inputs, ... }: {
  environment.systemPackages = [ inputs.distill.packages.${system}.default ];
}
```

### Devbox

> **Note:** Until distill is available in nixpkgs, install it via the GitHub
> flake reference.

```bash
# Install latest
devbox add github:damnhandy/distill#distill

# Pin to a specific version
devbox add github:damnhandy/distill/v0.1.0#distill
```

Or add directly to `devbox.json`:

```json
{
  "packages": [
    "github:damnhandy/distill/v0.1.0#distill"
  ]
}
```

### go install

```bash
go install github.com/damnhandy/distill@latest
```

> **Note:** Binaries installed this way report version `dev` — Go's toolchain does not
> support build-time version injection via `go install`. All other installation methods
> report the correct release version.

---

## Requirements

- macOS or Windows with Docker Desktop 3.0+, or Linux/WSL2 with Podman 3.0+
- `grype` — for `distill scan` (optional)
- `syft` — for `distill attest` (optional)
- `cosign` — for `distill provenance` (optional)
- `skopeo` — for base-image digest resolution in `distill provenance` (optional)

Run `distill doctor` to check your environment and get install instructions for any missing tools.

## Getting started

```bash
# Scaffold a new spec file
distill init --base ubi9 --name myapp

# Or for Debian
distill init --base debian --name myservice

# Build the CLI from source
go build -o distill .
```

## Usage

### Scaffold a spec file

```bash
# Scaffold with a known base distribution
distill init --base ubi9 --name myapp
distill init --base debian --name myservice --tag myregistry.io/myservice:latest
distill init --base ubuntu --variant dev --output dev.distill.yaml

# Available base values: ubi9, ubi8, fedora, debian, ubuntu, ubuntu22
```

### Build an image

Tags and platforms are declared in the spec file:

```yaml
tags:
  - myregistry.io/myapp:latest
platforms:
  - linux/amd64
  - linux/arm64
```

```bash
# Build all platforms defined in the spec
distill build --spec image.distill.yaml

# Override to build a single platform
distill build --spec image.distill.yaml --platform linux/arm64
```

### Scan for CVEs

```bash
distill scan myregistry.io/myapp:latest

# Fail on high severity or above
distill scan --fail-on high myregistry.io/myapp:latest
```

### Generate an SBOM

```bash
distill attest myregistry.io/myapp:latest
distill attest --output sbom.spdx.json myregistry.io/myapp:latest
```

### Attach SLSA provenance

```bash
# Minimal provenance (builder identity only)
distill provenance myregistry.io/myapp:latest

# Enriched provenance — includes spec digest, base-image digest, and parameters
distill provenance --spec image.distill.yaml myregistry.io/myapp:latest

# Save the predicate JSON for auditing
distill provenance --spec image.distill.yaml --predicate provenance.json myregistry.io/myapp:latest
```

Attestations use keyless Sigstore signing and are stored in the registry alongside the image.
Verify with:

```bash
cosign verify-attestation \
  --type slsaprovenance \
  --certificate-identity-regexp "https://github.com/damnhandy/distill" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  myregistry.io/myapp:latest

# Or verify the distill CLI binary itself
slsa-verifier verify-artifact \
  distill_linux_amd64.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/damnhandy/distill
```

## Supply-chain security

distill applies supply-chain security at two levels:

**Images you build with distill:**

| Artifact | Tool | Command |
|---|---|---|
| SPDX SBOM | syft | `distill attest <image>` |
| SLSA v0.2 provenance | cosign | `distill provenance <image>` |
| CVE scan | grype | `distill scan <image>` |

**The distill binary itself:**

Each GitHub Release includes:

- Cosign-signed `checksums.txt` (keyless, Sigstore)
- SPDX SBOM for each release archive
- SLSA Level 3 provenance (`multiple.intoto.jsonl`) generated by [`slsa-framework/slsa-github-generator`](https://github.com/slsa-framework/slsa-github-generator) in an isolated, ephemeral build environment

## Spec reference

Spec files use the `.distill.yaml` extension.

```yaml
name: string                    # required — image name
description: string             # optional

# variant controls whether the package manager is removed from the final image.
# "runtime" removes it (default); "dev" retains it for development images.
variant: runtime | dev

# OCI image references applied to the built image.
tags:
  - myregistry.io/myapp:latest

# Target build platforms. Defaults to [linux/amd64, linux/arm64] when omitted.
platforms:
  - linux/amd64
  - linux/arm64

base:
  image: string                 # required — OCI image ref for the build host
  releasever: string            # required — distro version ("9", "bookworm", etc.)
  packageManager: dnf | apt    # optional — inferred from base.image if omitted

contents:
  packages:                     # required — list of packages to install
    - string

accounts:                       # optional
  run-as: string                # user to run the container as (USER in Dockerfile)
  users:
    - name: string
      uid: int
      gid: int
      shell: string             # default: /sbin/nologin or /usr/sbin/nologin
      groups: [string]          # additional groups
  groups:
    - name: string
      gid: int
      members: [string]         # optional group members

environment:                    # optional — ENV in Dockerfile
  KEY: value

entrypoint: [string]            # optional — ENTRYPOINT in Dockerfile
cmd: [string]                   # optional — CMD in Dockerfile
work-dir: string                # optional — WORKDIR in Dockerfile

annotations:                    # optional — LABEL in Dockerfile
  org.opencontainers.image.source: https://github.com/example/myapp

volumes:                        # optional — VOLUME in Dockerfile
  - /data

ports:                          # optional — EXPOSE in Dockerfile
  - "8080/tcp"

# paths declares filesystem entries to create in the image chroot.
paths:
  - type: directory             # directory | file | symlink
    path: /app
    uid: 10001
    gid: 10001
    mode: "0755"
```

## Examples

See [`examples/`](./examples/):

- [`rhel9-runtime/`](./examples/rhel9-runtime/) — minimal RHEL9/UBI9 base, target ≤30MB
- [`debian-runtime/`](./examples/debian-runtime/) — minimal Debian Bookworm base

## Comparison

| | Google distroless | ubi9-micro | distill |
|---|---|---|---|
| Customizable packages | No | No | Yes |
| Declarative spec | No | No | Yes |
| Package manager removed | Yes | Yes | Yes |
| Audit trail (RPM/dpkg DB) | No | No | Yes |
| SBOM at build time | No | No | Yes |
| SLSA provenance | No | No | Yes |
| Multi-distro | No (Debian only) | No (RHEL only) | Yes |
| Self-hostable build | No | No | Yes |
