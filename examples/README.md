# Examples

Each subdirectory contains a `.distill.yaml` spec and a `test.yaml`
[container-structure-test](https://github.com/GoogleContainerTools/container-structure-test)
configuration.

| Example | Distribution | Target size | Notes |
|---|---|---|---|
| [`rhel9-runtime/`](./rhel9-runtime/) | RHEL9 / UBI9 | ≤30MB | Base layer for all RHEL9-derived images |
| [`debian-runtime/`](./debian-runtime/) | Debian Bookworm | ≤20MB | APT backend validation |

## Building an example

```bash
# Build all platforms declared in the spec
distill build --spec examples/rhel9-runtime/image.distill.yaml

# Build a single platform
distill build --spec examples/rhel9-runtime/image.distill.yaml --platform linux/amd64

# Run structure tests
container-structure-test test \
  --image distill-example-rhel9-runtime:latest \
  --config examples/rhel9-runtime/test.yaml
```
