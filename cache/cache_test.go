package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk"
)

func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		assert.NilError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		assert.NilError(t, os.WriteFile(p, []byte(content), 0o600))
	}
}

func TestPaths(t *testing.T) {
	t.Setenv("CARGO_TARGET_DIR", "")
	o := Options{CargoHome: "/home/cargo", Dir: "/project"}
	assert.DeepEqual(t, Paths(o), []string{
		filepath.Join("/home/cargo", "registry", "index"),
		filepath.Join("/home/cargo", "registry", "cache"),
		filepath.Join("/home/cargo", "git", "db"),
	})

	o.Bin, o.Targets = true, true
	assert.DeepEqual(t, Paths(o)[3:], []string{
		filepath.Join("/home/cargo", "bin"),
		filepath.Join("/project", "target"),
		filepath.Join("/home/cargo", ".crates.toml"),
		filepath.Join("/home/cargo", ".crates2.json"),
	})
}

func TestPrepare(t *testing.T) {
	t.Setenv("CARGO_TARGET_DIR", "")
	o := Options{CargoHome: t.TempDir(), Dir: t.TempDir(), Bin: true, Targets: true}
	assert.NilError(t, Prepare(o))
	for _, d := range Dirs(o) {
		info, err := os.Stat(d)
		assert.NilError(t, err)
		assert.Assert(t, info.IsDir())
	}
	_, err := os.Stat(filepath.Join(o.CargoHome, ".crates.toml"))
	assert.Assert(t, os.IsNotExist(err), "files cargo install owns must not be created")
}

func TestTargetDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	t.Setenv("CARGO_TARGET_DIR", "")
	assert.Equal(t, TargetDir(dir), filepath.Join(dir, "target"))

	t.Setenv("CARGO_TARGET_DIR", "out")
	assert.Equal(t, TargetDir(dir), filepath.Join(dir, "out"))

	abs := filepath.Join(t.TempDir(), "target")
	t.Setenv("CARGO_TARGET_DIR", abs)
	assert.Equal(t, TargetDir(dir), abs)
}

func TestKeys(t *testing.T) {
	save, restore := Keys(Options{Scope: "build", Key: "linux", Rustc: "20261013abcd"}, "deps")
	prefix := "setup-rust-v1-" + sdk.OSArch() + "-build-linux-20261013abcd-"
	assert.Equal(t, save, prefix+"deps")
	assert.DeepEqual(t, restore, []string{prefix + "deps", prefix})
}

func TestKeys_LeavesOutWhatIsNotSet(t *testing.T) {
	save, _ := Keys(Options{Rustc: "20261013abcd", Nonce: "n1"}, "deps")
	assert.Equal(t, save, "setup-rust-v1n1-"+sdk.OSArch()+"-20261013abcd-deps")
}

func TestHashDeps(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"Cargo.toml":              "[workspace]",
		"Cargo.lock":              "lock",
		"crates/a/Cargo.toml":     "[package]",
		"crates/a/src/lib.rs":     "fn a() {}",
		"target/debug/x.d":        "build output",
		".cargo/config.toml":      "[build]",
		"rust-toolchain.toml":     "[toolchain]",
		"node_modules/Cargo.toml": "not ours",
	})
	hash, found, err := HashDeps(dir)
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, len(hash), 20)

	t.Run("SourceAndBuildOutputDoNotChangeIt", func(t *testing.T) {
		write(t, dir, map[string]string{"crates/a/src/lib.rs": "fn b() {}", "target/debug/y.d": "more", "node_modules/Cargo.toml": "changed"})
		again, _, err := HashDeps(dir)
		assert.NilError(t, err)
		assert.Equal(t, again, hash)
	})

	for _, f := range []string{"Cargo.lock", "crates/a/Cargo.toml", ".cargo/config.toml", "rust-toolchain.toml"} {
		t.Run(strings.ReplaceAll(f, "/", "_")+"ChangesIt", func(t *testing.T) {
			before, _, err := HashDeps(dir)
			assert.NilError(t, err)
			write(t, dir, map[string]string{f: "changed " + f})
			after, _, err := HashDeps(dir)
			assert.NilError(t, err)
			assert.Assert(t, after != before)
		})
	}
}

func TestHashDeps_NoCargoToml(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"rust-toolchain.toml": "[toolchain]"})
	_, found, err := HashDeps(dir)
	assert.NilError(t, err)
	assert.Assert(t, !found)
}

func TestRustcHash(t *testing.T) {
	got, err := RustcHash("rustc 1.99.1 (58cf8f9c3 2026-10-13)\nbinary: rustc\ncommit-hash: 58cf8f9c3a2b1c0d\ncommit-date: 2026-10-13\nhost: x86_64-unknown-linux-gnu\n")
	assert.NilError(t, err)
	assert.Equal(t, got, "2026101358cf")

	_, err = RustcHash("rustc 1.99.1\n")
	assert.ErrorContains(t, err, "no commit-date or commit-hash")
}
