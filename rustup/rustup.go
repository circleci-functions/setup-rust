package rustup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Rustup runs one rustup binary.
type Rustup struct {
	// Bin is the rustup to run.
	Bin string
	// Stdout and Stderr receive the output of Run.
	Stdout, Stderr io.Writer
}

// Run runs rustup with args in dir, streaming its output.
func (r Rustup) Run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = r.Stdout, r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rustup %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Output runs rustup with args in dir and returns its standard output.
// Standard error streams as for Run.
func (r Rustup) Output(ctx context.Context, dir string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &out, r.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rustup %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}
