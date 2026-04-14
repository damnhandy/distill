# distill

Build minimal, immutable OCI images from enterprise Linux base distributions.

distill is a CLI tool that takes a declarative YAML spec and produces a minimal
`FROM scratch` OCI image using a chroot bootstrap strategy. It is a
distro-agnostic alternative to Google's [distroless](https://github.com/GoogleContainerTools/distroless)
images for teams that need images rooted in RHEL, UBI, Debian, or Ubuntu.

## How it works

1. Reads a declarative `image.yaml` spec describing the desired packages and configuration
2. Runs a privileged build container (`podman run --privileged`) using the target base image — so the correct package manager, repos, and release version are always available
3. Installs only the listed packages into an isolated chroot directory inside the container
4. After the container exits, the populated chroot is on the host filesystem
5. Copies the chroot into a `FROM scratch` container via `buildah add`
6. Commits the result as a minimal OCI image

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

## Requirements

- Linux host (or the provided devcontainer on macOS)
- `podman` — runs the privileged bootstrap container
- `buildah` — assembles the final FROM scratch image
- `grype` — for `distill scan` (optional)
- `syft` — for `distill attest` (optional)

All tools are available via `devbox shell`.

## Getting started

```bash
# Enter the dev environment (installs all tools on first run)
devbox shell

# Download Go module dependencies
go mod tidy

# Build the CLI
go build -o distill .

# Or install to $GOBIN
go install .
```

## Usage

### Build an image

```bash
distill build --spec examples/rhel9-runtime/image.yaml --tag myregistry.io/rhel9-runtime:latest

# Build for ARM64
distill build --spec examples/rhel9-runtime/image.yaml \
  --tag myregistry.io/rhel9-runtime:latest \
  --platform linux/arm64
```

### Scan for CVEs

```bash
distill scan myregistry.io/rhel9-runtime:latest

# Fail on high severity or above
distill scan --fail-on high myregistry.io/rhel9-runtime:latest
```

### Generate an SBOM

```bash
distill attest myregistry.io/rhel9-runtime:latest
distill attest --output sbom.spdx.json myregistry.io/rhel9-runtime:latest
```

## ImageSpec reference

```yaml
name: string                   # required — image name
description: string            # optional

base:
  image: string                # required — OCI image ref for the build host
  releasever: string           # required — distro version ("9", "bookworm", etc.)
  packageManager: dnf | apt   # optional — inferred from base.image if omitted

packages:                      # required — list of packages to install
  - string

accounts:                      # optional
  groups:
    - name: string
      gid: int
  users:
    - name: string
      uid: int
      gid: int
      shell: string            # default: /sbin/nologin
      groups: [string]         # additional groups

image:                         # optional — OCI image config
  cmd: [string]
  workdir: string
  env:
    KEY: value

immutable: bool                # default: true — remove package manager after install
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
| Multi-distro | No (Debian only) | No (RHEL only) | Yes |
| Self-hostable build | No | No | Yes |
