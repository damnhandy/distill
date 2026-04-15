package builder

import (
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

func fmtCmd(name string, args []string) string {
	return name + " " + strings.Join(args, " ")
}
