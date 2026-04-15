package builder

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// ContainerCLI represents the container runtime CLI to use for running containers.
type ContainerCLI string

const (
	// CLIPodman uses Podman for container operations — the default on Linux.
	CLIPodman ContainerCLI = "podman"
	// CLIDocker uses Docker via the Docker socket API — the default on macOS and Windows.
	CLIDocker ContainerCLI = "docker"
)

// DetectCLI returns the appropriate container CLI for the current OS.
//
// On macOS and Windows, Docker Desktop is used via the Docker socket API.
// On Linux (including WSL2 on Windows), Podman is used. WSL2 users get the
// Linux defaults automatically because runtime.GOOS reports "linux" inside WSL2.
//
// The DISTILL_CONTAINER_CLI environment variable overrides auto-detection.
// Valid values are "docker" and "podman". This is primarily useful in CI
// environments where both runtimes are available but one is preferred.
func DetectCLI() ContainerCLI {
	if override := os.Getenv("DISTILL_CONTAINER_CLI"); override != "" {
		return ContainerCLI(override)
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return CLIDocker
	default:
		return CLIPodman
	}
}

// requiredTools returns the tools that must be present on PATH for the given CLI.
//
// Both Docker and Podman now use their built-in multi-stage build support
// (`docker build` / `podman build`) to handle the full bootstrap and assembly
// pipeline. No separate buildah binary is needed.
func requiredTools(cli ContainerCLI) []string {
	if cli == CLIDocker {
		return []string{"docker"}
	}
	return []string{"podman"}
}

// checkToolsOnPath verifies that every tool in names is present on PATH.
func checkToolsOnPath(names []string) error {
	var missing []string
	for _, bin := range names {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tools not found on PATH: %v\n"+
			"Run 'distill doctor' for install instructions", missing)
	}
	return nil
}
