# Using distill in GitHub Actions

distill ships a composite action — `damnhandy/distill/.github/actions/setup-distill` — that installs the CLI on the runner and puts it on `PATH`. After that step, call `distill` commands directly.

## Prerequisites

distill builds images by running a **privileged Docker container** that uses chroot to install packages. GitHub Actions `ubuntu-latest` hosted runners have Docker pre-installed and support `--privileged` without extra configuration.

Required runner: `ubuntu-latest` (or any Linux runner with Docker and `--privileged` support).

> **Important:** On Linux, distill defaults to Podman. GitHub-hosted `ubuntu-latest` runners ship Docker, not Podman, so all workflow steps that invoke `distill build` or `distill publish` must set `DISTILL_CONTAINER_CLI: docker`.

## Basic usage

```yaml
- uses: damnhandy/distill/.github/actions/setup-distill@main
  with:
    version: v0.3.1   # pin to a release; omit for latest
```

After this step, `distill` is on `PATH` and you can call any subcommand.

## Example: build only (no push)

Useful for validating a spec on every pull request. Passes `--platform linux/amd64` to skip the arm64 build (which requires QEMU) and keeps the job fast.

```yaml
name: Build image

on:
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: damnhandy/distill/.github/actions/setup-distill@main
        with:
          version: v0.3.1

      - name: Build image
        env:
          DISTILL_CONTAINER_CLI: docker
        run: distill build --spec image.distill.yaml --platform linux/amd64
```

To build both platforms on a PR, add QEMU first:

```yaml
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - name: Build image (all platforms)
        env:
          DISTILL_CONTAINER_CLI: docker
        run: distill build --spec image.distill.yaml
```

## Example: full publish workflow with GHCR and SLSA provenance

This is the recommended pattern for a main-branch workflow. The `id-token: write` permission enables GitHub's OIDC token, which distill uses for keyless cosign signing when attaching SLSA provenance to the pushed image.

```yaml
name: Build and publish image

on:
  push:
    branches: [main]

permissions:
  contents: read
  packages: write    # push to GHCR
  id-token: write    # keyless SLSA provenance signing via OIDC

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: damnhandy/distill/.github/actions/setup-distill@main
        with:
          version: v0.3.1

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Publish image
        env:
          DISTILL_CONTAINER_CLI: docker
        run: distill publish --spec image.distill.yaml
```

`distill publish` runs the full pipeline in order: build → CVE scan → push → SBOM → SLSA provenance. Which steps execute is controlled by the `pipeline:` section of your spec file.

## Example: matrix build across multiple specs

```yaml
name: Build all distilled images

on:
  push:
    branches: [main]

permissions:
  contents: read
  packages: write
  id-token: write

jobs:
  publish:
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        spec:
          - examples/rhel9-runtime/image.distill.yaml
          - examples/debian-runtime/image.distill.yaml
          - examples/ubuntu-runtime/image.distill.yaml
    steps:
      - uses: actions/checkout@v6

      - uses: damnhandy/distill/.github/actions/setup-distill@main
        with:
          version: v0.3.1

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Publish ${{ matrix.spec }}
        env:
          DISTILL_CONTAINER_CLI: docker
        run: distill publish --spec ${{ matrix.spec }}
```

## Action inputs and outputs

| Input | Default | Description |
|---|---|---|
| `version` | `latest` | distill version to install (`v0.3.1`, `latest`) |

| Output | Description |
|---|---|
| `distill-version` | Version string of the installed binary |

## Pinning to a version

Using `@main` as the action ref picks up the latest action code. To pin to a stable release, use a version tag once one is available (e.g., `@v1`). During early development, `@main` with an explicit `version:` input for the binary is the recommended approach.

## GitLab CI

distill works in GitLab CI, but requires a runner with `privileged: true` in its configuration — GitLab.com shared runners do not enable this by default.

If you have a self-hosted runner with Docker executor and `privileged: true`, the setup is straightforward:

```yaml
variables:
  DISTILL_VERSION: v0.3.1

.distill-setup: &distill-setup
  before_script:
    - curl -sfL https://raw.githubusercontent.com/damnhandy/distill/main/scripts/install.sh | sh -s -- -b "$HOME/.local/bin" "$DISTILL_VERSION"
    - export PATH="$HOME/.local/bin:$PATH"

build-image:
  <<: *distill-setup
  image: docker:latest
  services:
    - docker:dind
  tags:
    - privileged
  script:
    - DISTILL_CONTAINER_CLI=docker distill build --spec image.distill.yaml
```

> **Note:** GitLab.com SaaS runners do not support `--privileged` builds. You must bring your own runner with this capability enabled.
