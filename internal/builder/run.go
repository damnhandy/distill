package builder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// run executes a command, streaming combined stdout+stderr to w.
func run(ctx context.Context, w io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", fmtCmd(name, args), err)
	}
	return nil
}

// capture runs a command and returns its trimmed stdout.
// Stderr is captured and included in the error message on failure.
func capture(ctx context.Context, name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w\n%s", fmtCmd(name, args), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func fmtCmd(name string, args []string) string {
	return name + " " + strings.Join(args, " ")
}
