package clitest_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestCLI_InstallsRustupThenStable(t *testing.T) {
	j := newJob(t)

	out, err := j.run()
	assert.NilError(t, err, "got: %s", out)
	assert.DeepEqual(t, j.calls(), []string{
		"rustup-init -y --no-modify-path --default-toolchain none",
		"rustup toolchain install stable --profile minimal --no-self-update",
		"rustup default stable",
		"rustup override set stable",
		"rustup show active-toolchain",
	})
	assert.Assert(t, strings.Contains(out, "toolchain stable is active"), "got: %s", out)
	assert.Assert(t, strings.Contains(out, "cargo and rustc resolve from "+j.sandbox.BinDir), "got: %s", out)

	assert.Equal(t, j.later(j.wd, "cargo", "--version"), "cargo 1.0.0-fake (stable)")
	assert.Equal(t, j.later(t.TempDir(), "rustc", "--version"), "rustc 1.0.0-fake (stable)",
		"a fresh rustup gets the toolchain as its default, so it works outside --rust-src-dir too")
}

func TestCLI_PassesTheFlagsToRustup(t *testing.T) {
	j := newJob(t)

	out, err := j.run("--toolchain", "nightly", "--components", "rustfmt, clippy", "--target", "wasm32-unknown-unknown", "--no-override")
	assert.NilError(t, err, "got: %s", out)
	calls := j.calls()
	assert.Assert(t, slices.Contains(calls,
		"rustup toolchain install nightly --target wasm32-unknown-unknown --component rustfmt --component clippy --profile minimal --allow-downgrade --no-self-update"),
		"got: %v", calls)
	assert.Assert(t, !slices.ContainsFunc(calls, func(c string) bool { return strings.HasPrefix(c, "rustup override") }),
		"--no-override must not set an override; got: %v", calls)
}

func TestCLI_InstallsWhatTheToolchainFileNames(t *testing.T) {
	j := newJob(t)
	j.write("rust-toolchain.toml", "[toolchain]\nchannel = \"1.83\"\ncomponents = [\"rustfmt\"]\n")

	out, err := j.run()
	assert.NilError(t, err, "got: %s", out)
	assert.DeepEqual(t, j.calls(), []string{
		"rustup-init -y --no-modify-path --default-toolchain none",
		"rustup toolchain install",
		"rustup show active-toolchain",
		"rustup default 1.83",
	})
	assert.Equal(t, j.later(j.wd, "cargo", "--version"), "cargo 1.0.0-fake (1.83)")
}

func TestCLI_TheToolchainFlagWinsOverAToolchainFile(t *testing.T) {
	j := newJob(t)
	j.write("rust-toolchain", "1.83\n")

	out, err := j.run("--toolchain", "beta")
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, j.later(j.wd, "cargo", "--version"), "cargo 1.0.0-fake (beta)")
}

func TestCLI_RustSrcDir(t *testing.T) {
	j := newJob(t)
	j.write("crates/app/rust-toolchain.toml", "[toolchain]\nchannel = \"1.83\"\n")
	app := filepath.Join(j.wd, "crates", "app")

	out, err := j.run("--rust-src-dir", "crates/app")
	assert.NilError(t, err, "got: %s", out)
	for _, c := range j.callsIn() {
		if strings.HasPrefix(c[0], "rustup ") {
			assert.Equal(t, realPath(t, c[1]), realPath(t, app), "%s ran in the wrong directory", c[0])
		}
	}
	assert.Equal(t, j.later(app, "cargo", "--version"), "cargo 1.0.0-fake (1.83)")
}

func TestCLI_RustSrcDirMustExist(t *testing.T) {
	j := newJob(t)

	out, err := j.run("--rust-src-dir", "missing")
	assert.Assert(t, err != nil, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "rust-src-dir"), "got: %s", out)
	assert.Equal(t, len(j.calls()), 0)
}

func TestCLI_UsesTheRustupAlreadyOnPath(t *testing.T) {
	j := newJob(t)
	j.putRustupOnPath("1.80")

	out, err := j.run()
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, j.dist.requests.Load(), int32(0), "rustup-init must not be downloaded")
	assert.DeepEqual(t, j.calls(), []string{
		"rustup toolchain install stable --profile minimal --no-self-update",
		"rustup override set stable",
		"rustup show active-toolchain",
	})
	assert.Equal(t, j.later(t.TempDir(), "cargo", "--version"), "cargo 1.0.0-fake (1.80)",
		"the image's own default toolchain is left as it was")
}

func TestCLI_RejectsARustupInitThatFailsItsChecksum(t *testing.T) {
	j := newJob(t)
	j.dist.checksum = strings.Repeat("0", 64)

	out, err := j.run()
	assert.Assert(t, err != nil, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "SHA-256"), "got: %s", out)
	assert.Equal(t, len(j.calls()), 0, "rustup-init must not run")
}

func TestCLI_UnknownFlag(t *testing.T) {
	j := newJob(t)
	_, err := j.run("--no-such-flag")
	assert.Assert(t, err != nil)
}

func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	assert.NilError(t, err)
	return r
}
