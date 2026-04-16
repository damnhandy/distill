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

| Distribution | Package Manager | Example `source.image` |
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
- `grype` — for `distill scan`, `distill publish`, and `distill build --pipeline` (optional)
- `syft` — for `distill attest`, `distill publish`, and `distill build --pipeline` (optional)
- `cosign` — for `distill provenance` and `distill publish` (optional)
- `skopeo` — for base-image digest resolution in provenance (optional)

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
distill init --base debian --name myservice --destination myregistry.io/myservice:latest
distill init --base ubuntu --variant dev --output dev.distill.yaml

# Available base values: ubi9, ubi8, fedora, debian, ubuntu, ubuntu22
```

### Build an image

The destination image and platforms are declared in the spec file:

```yaml
destination:
  image: myregistry.io/myapp
  releasever: latest        # optional — defaults to "latest"
platforms:
  - linux/amd64
  - linux/arm64
```

```bash
# Build all platforms defined in the spec
distill build --spec image.distill.yaml

# Override to build a single platform
distill build --spec image.distill.yaml --platform linux/arm64

# Build and run pipeline steps (scan, sbom) on the local image
distill build --spec image.distill.yaml --pipeline
```

### Publish an image

`distill publish` is the full deployment workflow. It runs in order:

1. Build all platforms
2. Scan for CVEs — fails before pushing a vulnerable image
3. Push to the registry
4. Generate SBOM
5. Attach SLSA provenance

Which steps run is controlled by the `pipeline:` section of the spec (see [Spec reference](#spec-reference)). Steps 2, 4, and 5 are skipped when the corresponding pipeline entry is absent or disabled.

```bash
# Full workflow: build → scan → push → sbom → provenance
distill publish --spec image.distill.yaml

# Push only (skip all pipeline steps)
distill publish --spec image.distill.yaml --skip-pipeline

# Skip build (push and run pipeline on an already-built local image)
distill publish --spec image.distill.yaml --skip-build

# Publish a single platform only
distill publish --spec image.distill.yaml --platform linux/amd64
```

### Scan, attest, and provenance (standalone)

These commands operate on any OCI image reference — useful for one-off inspection or images not built with distill.

```bash
# Scan for CVEs
distill scan myregistry.io/myapp:latest
distill scan --fail-on high myregistry.io/myapp:latest

# Generate an SBOM
distill attest myregistry.io/myapp:latest
distill attest --output sbom.spdx.json myregistry.io/myapp:latest

# Attach SLSA provenance
distill provenance myregistry.io/myapp:latest
distill provenance --spec image.distill.yaml myregistry.io/myapp:latest
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

| Artifact | Tool | Automated | Standalone |
|---|---|---|---|
| CVE scan | grype | `distill publish` / `distill build --pipeline` | `distill scan <image>` |
| SPDX SBOM | syft | `distill publish` / `distill build --pipeline` | `distill attest <image>` |
| SLSA v0.2 provenance | cosign | `distill publish` | `distill provenance <image>` |

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

# Target build platforms. Defaults to [linux/amd64, linux/arm64] when omitted.
platforms:
  - linux/amd64
  - linux/arm64

source:
  image: string                 # required — OCI image ref for the build host
  releasever: string            # required — distro version ("9", "bookworm", etc.)
  packageManager: dnf | apt    # optional — inferred from source.image if omitted

# Destination OCI image reference for the built image. Optional.
destination:
  image: string                 # registry/name (e.g. myregistry.io/myapp)
  releasever: string            # tag to apply; defaults to "latest" when omitted

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

# pipeline declares supply-chain steps that run after distill build --pipeline
# or distill publish. Omit any sub-section to disable that step.
pipeline:
  scan:
    enabled: true | false
    fail-on: critical           # optional — critical | high | medium | low | negligible
  sbom:
    enabled: true | false
    output: sbom.spdx.json      # optional — path for the SPDX JSON file
  provenance:
    enabled: true | false
    predicate: string           # optional — path to write predicate JSON for auditing
```

## Examples

See [`examples/`](./examples/):

- [`rhel9-runtime/`](./examples/rhel9-runtime/) — minimal RHEL9/UBI9 base, target ≤30MB
- [`debian-runtime/`](./examples/debian-runtime/) — minimal Debian Bookworm base

## Comparison

| | Google distroless | ubi9-micro | Docker Hardened Images | distill |
|---|---|---|---|---|
| Customizable packages | No | No | No | Yes |
| Declarative spec | No | No | No | Yes |
| Package manager removed | Yes | Yes | Yes | Yes |
| Audit trail (RPM/dpkg DB) | No | No | Yes | Yes |
| SBOM at build time | No | No | Yes | Yes |
| SLSA provenance | No | No | Yes | Yes |
| Multi-distro | No (Debian only) | No (RHEL only) | No (Wolfi/Alpine only) | Yes |
| Self-hostable build | No | No | No | Yes |
