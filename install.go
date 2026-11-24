package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-rust/cache"
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

	Cache          bool   `default:"true" negatable:"" help:"Use the job cache: restore cargo's registry and git checkouts now, with the target directory and CARGO_HOME/bin as below, and save them at the end of the job. false turns off all of it."`
	CacheTargets   bool   `default:"true" negatable:"" help:"Include the target directory in --rust-src-dir, or $CARGO_TARGET_DIR."`
	CacheBin       bool   `default:"true" negatable:"" help:"Include CARGO_HOME/bin, where cargo install puts binaries."`
	CacheOnFailure bool   `default:"true" negatable:"" help:"Save the cache even when the job fails."`
	CacheSaveIf    bool   `name:"cache-save-if" default:"true" negatable:"" help:"Save the cache at the end of the job. false only restores it."`
	CacheKey       string `help:"Added to the cache key, to keep apart jobs that would otherwise share a cache."`
	CacheSharedKey string `help:"Used in the cache key in place of the job name, so several jobs share one cache."`

	BinDir     string `hidden:"" help:"Link the rustup proxies into this directory instead of discovering one on PATH."`
	CacheNonce string `hidden:"" help:"Partition the cache, so a test run cannot restore an earlier run's."`
}

func (c *cli) Run(ctx context.Context, client *sdk.Client) error {
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

	if c.Cache {
		c.setUpCache(ctx, client, r, active, dir)
	}

	res, err := rustpath.Link(rustpath.Options{CargoBin: filepath.Dir(bin), BinDir: c.BinDir})
	if err != nil {
		return err
	}
	logf("cargo and rustc resolve from %s", res.Dir)
	return nil
}

// setUpCache restores the cache and registers its save for the end of the job,
// so a build in between needs no cache steps of its own. It runs before the
// proxies are linked, since restoring CARGO_HOME/bin rewrites them. Nothing
// here fails the job: a cache is best effort.
func (c *cli) setUpCache(ctx context.Context, client *sdk.Client, r rustup.Rustup, active, dir string) {
	deps, found, err := cache.HashDeps(dir)
	if err != nil {
		logf("cache skipped: %v", err)
		return
	}
	if !found {
		logf("cache skipped: no Cargo.toml under %s", dir)
		return
	}
	verbose, err := r.Output(ctx, dir, "run", active, "rustc", "-vV")
	if err != nil {
		logf("cache skipped: %v", err)
		return
	}
	rustc, err := cache.RustcHash(verbose)
	if err != nil {
		logf("cache skipped: %v", err)
		return
	}
	home, err := rustup.CargoHome()
	if err != nil {
		logf("cache skipped: %v", err)
		return
	}

	o := cache.Options{
		CargoHome: home,
		Dir:       dir,
		Targets:   c.CacheTargets,
		Bin:       c.CacheBin,
		Key:       c.CacheKey,
		Scope:     cmp.Or(c.CacheSharedKey, os.Getenv("CIRCLE_JOB")),
		Rustc:     rustc,
		Nonce:     c.CacheNonce,
	}
	save, restore := cache.Keys(o, deps)
	_, err = client.RestoreCache(ctx, restore, sdk.WithStepName("Restoring Rust cache"))
	sdk.LogCacheErr("restore Rust", err)
	if !c.CacheSaveIf {
		return
	}
	if err := cache.Prepare(o); err != nil {
		logf("cache not saved: %v", err)
		return
	}
	teardown := client.AtTeardown()
	if !c.CacheOnFailure {
		teardown = teardown.OnSuccess()
	}
	err = teardown.SaveCache(ctx, save, cache.Paths(o), sdk.WithStepName("Saving Rust cache"))
	sdk.LogCacheErr("save Rust", err)
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
