<p align="center">
  <img src="assets/images/distill-icon.jpg" alt="distill" width="300" />
</p>

# Distill

<p align="center">
  <a href="https://github.com/damnhandy/distill/releases/latest"><img src="https://img.shields.io/github/v/release/damnhandy/distill?label=release&color=blue" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="Apache 2.0 license"></a>
</p>

What if you could strip an enterprise Linux image down to only what your application actually needs — and nothing else?

`distill` is a CLI tool that takes a declarative `.distill.yaml` spec and produces a minimal `FROM scratch` OCI image using a chroot bootstrap strategy. The result — a **distilled image** — contains only the packages you listed, runs as a non-root user by default, and ships with a CVE scan, SBOM, and SLSA provenance baked into the build process. It is a self-hostable, distro-agnostic alternative to [Google distroless](https://github.com/GoogleContainerTools/distroless) or [Docker Hardened Images](https://www.docker.com/products/hardened-images/) for teams that need images rooted in RHEL, UBI, Debian, or Ubuntu.

This project started as a collection of shell scripts that used [Buildah](https://buildah.io/) to build images. The scripts produced small images, but Buildah wasn't the reason — the real trick was using chroot to install only the bare essentials into an isolated directory, then copying that into a `FROM scratch` image. Distill takes that same idea and wraps it into a simple, repeatable process.

## How much smaller?

| Base image | Size | Distilled image | Size | Reduction |
|---|---|---|---|---|
| `docker.io/redhat/ubi9` | ~214 MB | `rhel9-distilled` | ~28 MB | ~87% |
| `debian:bookworm-slim` | ~74 MB | `debian-distilled` | ~17 MB | ~77% |

> Sizes are uncompressed. Results vary by package selection — see [`examples/`](./examples/) for reproducible specs.

## How it works

1. Reads your `.distill.yaml` spec to know which packages and configuration you want
2. Spins up a multi-stage Docker/Podman build using your chosen base image — so the correct package manager, repos, and release version are always in play
3. Installs only the packages you listed into an isolated chroot inside the builder stage
4. Copies that chroot into a `FROM scratch` final stage — the package manager never makes it in
5. Produces a lean, immutable distilled image ready to tag and push

The package manager is never present in the final image — not removed as a layer, but never copied in to begin with. This is the same reason Chainguard built [`apko`](https://github.com/chainguard-dev/apko) rather than using Dockerfiles; distill is the equivalent for RPM and APT-based enterprise distributions.

## ⚠️ Disclaimer

> 🤖 **This project is an experiment.** `distill` is being built almost entirely by AI agents working from a written spec and a prototype — with little to no human actively writing code. It is an exploration of what agentic software development looks like in practice: can a useful, production-quality tool emerge from a spec, a prototype, and a swarm of agents?
>
> 🚧 **Early development.** This project is in its early stages. The CLI interface, the `.distill.yaml` spec schema, and any public APIs are all **subject to change without notice** as the project evolves. Do not rely on the current interface for production workloads until a stable release is tagged.

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

We recommend [Devbox](https://www.jetify.com/docs/devbox) — it is what we use day-to-day and gives you a fully isolated dev environment. [Nix](https://nixos.org/) installs are supported too, since Devbox is built on top of it. If you just want the binary without the isolation layer, [Homebrew](https://brew.sh) works great on macOS and Linux.

### Devbox

> **Note:** Until distill is available in nixpkgs, install it via the GitHub flake reference.

```bash
# Install latest
devbox add github:damnhandy/distill#distill

# Pin to a specific version
devbox add github:damnhandy/distill/v0.2.0#distill
```

Or add directly to `devbox.json`:

```json
{
  "packages": [
    "github:damnhandy/distill/v0.2.0#distill"
  ]
}
```

### Nix / NixOS

```bash
# Install to your profile
nix profile install github:damnhandy/distill

# Pin to a specific version
nix profile install github:damnhandy/distill/v0.2.0

# Run without installing
nix run github:damnhandy/distill -- --help
```

For NixOS, add to your `configuration.nix`:

```nix
{ inputs, ... }: {
  environment.systemPackages = [ inputs.distill.packages.${system}.default ];
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
curl -sfL https://raw.githubusercontent.com/damnhandy/distill/main/scripts/install.sh | sh -s -- v0.2.0
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

## Requirements

- macOS or Windows with Docker Desktop 3.0+, or Linux/WSL2 with Podman 3.0+
- `grype` — for `distill scan`, `distill publish`, and `distill build --pipeline` (optional)
- `syft` — for `distill attest`, `distill publish`, and `distill build --pipeline` (optional)
- `cosign` — for `distill provenance` and `distill publish` (optional)
- `skopeo` — for base-image digest resolution in provenance (optional)

Run `distill doctor` to check your environment and get install instructions for any missing tools.

## Getting started

The fastest path to your first distilled image:

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

These commands work on any OCI image reference — handy for one-off inspection or images not built with distill.

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

Shipping a small image is only half the story — you also need to know what's in it and be able to prove it. Every distilled image you build with distill gets a CVE scan, an SPDX SBOM, and SLSA provenance attached automatically as part of `distill publish`.

**Distilled images you build:**

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

See [`examples/`](./examples/) for complete, working specs:

- [`rhel9-runtime/`](./examples/rhel9-runtime/) — minimal RHEL9/UBI9 distilled image, target ≤30 MB
- [`debian-runtime/`](./examples/debian-runtime/) — minimal Debian Bookworm distilled image, target ≤20 MB

## Comparison

How distill compares to other approaches to building minimal container images:

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
