package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/circleci-functions/setup-rust/rustpath"
	"github.com/circleci-functions/setup-rust/rustup"
	"github.com/circleci-functions/setup-rust/toolchain"
)

type cli struct {
	Toolchain  string `help:"Rust toolchain to install, e.g. stable, 1.83 or nightly-2026-01-01. Empty installs the one the rust-toolchain.toml or rust-toolchain in --rust-src-dir names, else stable." example:"stable"`
	Target     string `help:"Targets to add, comma separated, e.g. wasm32-unknown-unknown."`
	Components string `help:"Components to add, comma separated, e.g. rustfmt,clippy. A toolchain named by --toolchain, or stable by default, is installed with the minimal profile, so rustfmt and clippy come only when listed here or in rust-toolchain.toml." example:"rustfmt,clippy"`
	Override   bool   `default:"true" negatable:"" help:"Make the toolchain the one used in --rust-src-dir, with rustup override set. Ignored when it comes from a rust-toolchain file."`
	RustSrcDir string `name:"rust-src-dir" help:"Directory of the Rust project, where a rust-toolchain file is read and the override is set. Empty is the working directory."`

	BinDir string `hidden:"" help:"Link the rustup proxies into this directory instead of discovering one on PATH."`
}

func (c *cli) Run(ctx context.Context) error {
	dir, err := c.srcDir()
	if err != nil {
		return err
	}
	bin, fresh, err := ensureRustup(ctx)
	if err != nil {
		return err
	}

	r := rustup.Rustup{Bin: bin, Stdout: os.Stdout, Stderr: os.Stderr}
	active, err := toolchain.Install(ctx, r, toolchain.Request{
		Dir:        dir,
		Toolchain:  strings.TrimSpace(c.Toolchain),
		Targets:    splitList(c.Target),
		Components: splitList(c.Components),
		Override:   c.Override,
		SetDefault: fresh,
	})
	if err != nil {
		return err
	}
	logf("toolchain %s is active in %s", active, dir)

	res, err := rustpath.Link(rustpath.Options{CargoBin: filepath.Dir(bin), BinDir: c.BinDir})
	if err != nil {
		return err
	}
	logf("cargo and rustc resolve from %s", res.Dir)
	return nil
}

// srcDir returns --rust-src-dir as an absolute path, after checking it is a
// directory.
func (c *cli) srcDir() (string, error) {
	dir, err := filepath.Abs(c.RustSrcDir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("rust-src-dir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("rust-src-dir %s is not a directory", dir)
	}
	return dir, nil
}

// ensureRustup returns the rustup to use, installing one when the job has
// none. fresh reports that it was just installed, so has no default toolchain.
func ensureRustup(ctx context.Context) (bin string, fresh bool, err error) {
	if bin, ok := rustup.Find(); ok {
		logf("using the rustup at %s", bin)
		return bin, false, nil
	}
	triple, err := rustup.HostTriple()
	if err != nil {
		return "", false, err
	}
	logf("no rustup found, so installing it for %s", triple)
	bin, err = rustup.Install(ctx, rustup.InstallOptions{Triple: triple, Stdout: os.Stdout, Stderr: os.Stderr})
	return bin, err == nil, err
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "setup-rust: "+format+"\n", args...)
}

// splitList splits a comma-separated flag value, dropping blanks.
func splitList(s string) []string {
	var out []string
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
