# Agent Guidelines for distill

## What this project is

`distill` is a Go CLI tool that builds minimal, immutable OCI images from
enterprise Linux base distributions. It works by running a privileged
bootstrap container (`podman run --privileged`), populating a chroot directory
using the native package manager, and committing that directory into a
`FROM scratch` image via `buildah add`. The package manager is never present
in the output image.

Two backends are supported, selected automatically from `base.image` or set
explicitly with `base.packageManager`:

- **DNF** — RHEL, UBI, CentOS Stream, Rocky Linux, AlmaLinux, Fedora
- **APT** — Debian, Ubuntu (uses `debootstrap`)

## Repository layout

```text
cmd/                    # cobra CLI commands (build, scan, attest)
internal/
  spec/                 # ImageSpec YAML types, Parse(), validation
  builder/              # Builder interface, DNF/APT backends, OCI assembly
examples/               # ready-to-use ImageSpec YAML files + structure tests
```

## Build and run

```bash
devbox shell            # enter the dev environment (installs all tools)
go mod tidy             # resolve dependencies
go build -o distill .   # compile the binary
./distill --help
```

## Testing

Every non-trivial Go function must have a corresponding test. Run the full
suite with:

```bash
go test ./...
```

### Testing rules

- Use [testify](https://github.com/stretchr/testify) for all assertions —
  never use bare `t.Errorf` or `t.Fatalf` comparisons.
- Use `require.*` when a failure would make subsequent assertions meaningless
  (typically: after parsing, after error checks that gate further assertions).
- Use `assert.*` for independent checks within the same test.
- Use table-driven tests (`[]struct{ name string; ... }`) for functions with
  multiple input/output cases.
- Name sub-tests descriptively: `t.Run("missing name field", ...)` not
  `t.Run("test2", ...)`.
- Test files for unexported helpers use `package <pkg>` (internal); test files
  for exported APIs use `package <pkg>_test` (external/black-box).
- Do not add tests that only test language features or standard library
  behaviour. Test the logic this project owns.

### Testifylint

All test code is validated by
[testifylint](https://github.com/Antonboom/testifylint) via golangci-lint.
Common violations to avoid:

```go
// WRONG — use assert.True / assert.False for boolean conditions
assert.Equal(t, true, ok)
assert.Equal(t, false, found)

// WRONG — use require when subsequent assertions depend on this one
assert.NoError(t, err)
result.DoSomething()  // panics if err != nil

// WRONG — compare errors with assert.Equal
assert.Equal(t, someErr, err)  // use assert.ErrorIs or require.ErrorIs

// CORRECT
assert.True(t, ok)
require.NoError(t, err)
assert.ErrorIs(t, err, ErrSomeExpectedError)
```

### What is and is not testable

Functions that shell out to `podman` or `buildah` (`Build`, `Scan`, `Attest`,
`run`, `capture`, `assemble`) require a Linux runtime with those tools present.
They are integration tests, not unit tests. Mark them with `//go:build
integration` and do not run them in `go test ./...` by default.

The following are fully unit-testable and must have tests:

- `spec.Parse` — all valid inputs, all validation error paths
- `spec.inferPackageManager` — all known prefixes and the default fallback
- `spec.ImageSpec.IsImmutable` — nil pointer case and both explicit values
- `dnfBootstrapScript` — package list, accounts, immutable flag on/off
- `aptBootstrapScript` — package list, accounts, immutable flag on/off

## Code formatting

**All Go source files must be formatted with
[goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) before
committing.** `goimports` is a superset of `gofmt` that also organises import
blocks into three groups in order:

1. Standard library
2. Third-party packages
3. Internal packages (`github.com/damnhandy/distill/...`)

Install and run:

```bash
go install golang.org/x/tools/cmd/goimports@latest
goimports -w .
```

Code that is not formatted with `goimports` will be rejected by `golangci-lint`
in CI (`goimports` linter is enabled in `.golangci.yml`).

## Linting

```bash
golangci-lint run ./...
```

The linter configuration is in `.golangci.yml`. Do not disable linters inline
(`//nolint`) without a comment explaining why.

## Adding a new distribution backend

1. Create `internal/builder/<name>.go` with a struct that implements `Builder`.
2. Add the package manager identifier string to the `New()` switch in
   `internal/builder/builder.go`.
3. Add the image prefix(es) to `inferPackageManager()` in
   `internal/spec/spec.go`.
4. Add an example spec to `examples/<distro>-runtime/image.yaml`.
5. Write unit tests for the script generator in
   `internal/builder/<name>_test.go`.

## What not to do

- Do not add error handling for conditions that cannot happen in normal program
  flow. Trust the validation in `spec.Parse`.
- Do not modify `go.sum` by hand; run `go mod tidy`.
- Do not commit the compiled `distill` binary — it is in `.gitignore`.
- Do not use `//nolint` to suppress `testifylint` or `goimports` findings —
  fix the code instead.
- Do not write tests for the `run` and `capture` helpers directly; they are
  thin wrappers over `os/exec` and are covered by integration tests.
