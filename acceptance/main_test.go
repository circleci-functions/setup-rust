//go:build acceptance

// Package acceptance_test exercises setup-rust against real infrastructure: a
// real rustup-init from static.rust-lang.org and real toolchains. Tagged out
// of task test because it needs network. The CLI contract itself is covered
// hermetically by clitest/.
package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/circleci-functions/toolkit/testing/compiler"
)

// binaryPathEnvVar names a prebuilt setup-rust to test, for an executor with
// no Go toolchain to compile one with. The smoke jobs set it, and pair it with
// a cross-compiled copy of this suite. A relative path is resolved here, since
// the suite runs the binary from a working directory of its own.
const binaryPathEnvVar = "SETUP_RUST_BINARY"

var binaryPath string

func TestMain(m *testing.M) {
	status, err := runTests(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(status)
}

func runTests(m *testing.M) (int, error) {
	if bin := os.Getenv(binaryPathEnvVar); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		binaryPath = abs
		return m.Run(), nil
	}

	p := compiler.NewParallel(1)
	defer p.Cleanup()
	p.Add(compiler.Work{Result: &binaryPath, Name: "setup-rust", Target: "..", Source: "."})
	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}
	return m.Run(), nil
}
