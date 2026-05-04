# Copilot instructions for distill

## What this project does

distill is a Go CLI tool that builds minimal, immutable OCI images from enterprise Linux base distributions (RHEL/UBI, Debian, Ubuntu). It uses a chroot bootstrap strategy: it runs a **privileged container** using the target distribution's package manager, installs only the declared packages into an isolated chroot directory, then commits that chroot into a `FROM scratch` OCI image. The package manager is never present in the final image.

## Execution model — critical context for code review

**All build operations run inside containers, not on the local filesystem.**

When reviewing Go code in `internal/builder/`, be aware that:

- `exec.Command("podman", ...)` and `exec.Command("docker", ...)` calls invoke the container runtime to run privileged bootstrap containers. These are intentional and necessary — they are not shell injection risks in context, because the arguments are constructed programmatically from validated spec fields, not from user-supplied raw strings.
- Functions like `run()` and `capture()` in the builder package execute commands inside containers or against the container runtime daemon, not against the local filesystem.
- `buildah` commands assemble the final OCI image from the chroot contents. These also run against the container runtime, not the local disk.
- The `--privileged` flag on container runs is required by design — the chroot bootstrap must be root inside the container.

Do **not** suggest replacing `exec.Command` calls with Go filesystem APIs (e.g., `os.WriteFile`, `filepath.Walk`) in the builder package. The target filesystem is inside a container layer, not a local path.

## Runtime selection

distill auto-selects the container runtime via `DetectCLI()`:

- **Linux**: defaults to `podman`
- **macOS / Windows**: defaults to `docker`
- Override: set `DISTILL_CONTAINER_CLI=docker` or `DISTILL_CONTAINER_CLI=podman`

In GitHub Actions (`ubuntu-latest`), Podman is not installed. Workflows must set `DISTILL_CONTAINER_CLI: docker`. This env var is required in any step that calls `distill build` or `distill publish` on a hosted Linux runner.

## Testing boundaries

- Functions that shell out to `podman`, `docker`, or `buildah` are **integration tests**. They require a real Linux runtime and are tagged `//go:build integration`. They are excluded from `go test ./...`.
- Unit-testable code lives in `internal/spec/` (parsing, validation) and the script generators in `internal/builder/`.
- Do not suggest mocking the container runtime in unit tests — the project explicitly avoids this to prevent mock/prod divergence.

## Supply-chain security

Every image built by distill gets a CVE scan (grype), SPDX SBOM (syft), and SLSA provenance (cosign). `fail-on: critical` in scan configuration is intentional — it is a gate, not a suggestion. Do not suggest relaxing scan thresholds as a general fix; the correct remediation is updating the vulnerable packages.

## Spec files

User-facing image specs use `.distill.yaml` extension. The spec schema is defined in `internal/spec/`. The `source.image` field is a base distribution image used only for the build-time bootstrap — it is never present in the output image.
