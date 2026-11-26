package clitest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// cargoProject gives the job a project to key the cache on.
func (j *job) cargoProject() {
	j.t.Helper()
	j.write("Cargo.toml", "[package]\nname = \"hello\"\nversion = \"0.1.0\"\n")
	j.write("Cargo.lock", "version = 4\n")
}

// build stands in for the job's build: it leaves what each cached path holds,
// and returns the files it wrote.
func (j *job) build() []string {
	j.t.Helper()
	cargoHome := filepath.Join(j.sandbox.Home, ".cargo")
	files := []string{
		filepath.Join(cargoHome, "registry", "index", "index.crates.io", "config.json"),
		filepath.Join(cargoHome, "registry", "cache", "index.crates.io", "serde-1.0.0.crate"),
		filepath.Join(cargoHome, "git", "db", "repo", "HEAD"),
		filepath.Join(cargoHome, "bin", exe("cargo-nextest")),
		filepath.Join(cargoHome, ".crates.toml"),
		filepath.Join(cargoHome, ".crates2.json"),
		filepath.Join(j.wd, "target", "debug", "hello"),
	}
	for _, f := range files {
		assert.NilError(j.t, os.MkdirAll(filepath.Dir(f), 0o750))
		assert.NilError(j.t, os.WriteFile(f, []byte(filepath.Base(f)), 0o600))
	}
	return files
}

// freshExecutor removes what build wrote, as the next job's executor would
// not have it.
func (j *job) freshExecutor() {
	j.t.Helper()
	assert.NilError(j.t, os.RemoveAll(filepath.Join(j.sandbox.Home, ".cargo", "registry")))
	assert.NilError(j.t, os.RemoveAll(filepath.Join(j.sandbox.Home, ".cargo", "git")))
	assert.NilError(j.t, os.RemoveAll(filepath.Join(j.sandbox.Home, ".cargo", "bin", exe("cargo-nextest"))))
	assert.NilError(j.t, os.RemoveAll(filepath.Join(j.wd, "target")))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestCLI_CacheRoundTrip(t *testing.T) {
	j := newJob(t)
	j.cargoProject()
	j.env["CIRCLE_JOB"] = "lint"

	out, err := j.run()
	assert.NilError(t, err, "got: %s", out)
	pending := j.sandbox.Cache.PendingCacheSaves()
	assert.Equal(t, len(pending), 1, "got: %v", pending)
	assert.Assert(t, strings.HasPrefix(pending[0], "setup-rust-v1"+j.sandbox.CacheKeyNonce+"-"), "got: %s", pending[0])
	assert.Assert(t, strings.Contains(pending[0], "-lint-"), "the job name scopes the key; got: %s", pending[0])

	files := j.build()
	j.sandbox.Cache.EndJob(t)
	assert.DeepEqual(t, j.sandbox.Cache.SavedKeys(), pending)

	j.freshExecutor()
	out, err = j.run()
	assert.NilError(t, err, "got: %s", out)
	for _, f := range files {
		assert.Assert(t, exists(f), "expected %s to be restored", f)
	}
}

func TestCLI_CacheLeavesOutTargetsAndBinWhenAsked(t *testing.T) {
	j := newJob(t)
	j.cargoProject()

	out, err := j.run("--no-cache-targets", "--no-cache-bin")
	assert.NilError(t, err, "got: %s", out)
	files := j.build()
	j.sandbox.Cache.EndJob(t)

	j.freshExecutor()
	out, err = j.run("--no-cache-targets", "--no-cache-bin")
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, exists(files[1]), "the registry is always cached")
	assert.Assert(t, !exists(files[3]), "--no-cache-bin must leave out CARGO_HOME/bin")
	assert.Assert(t, !exists(files[6]), "--no-cache-targets must leave out the target directory")
}

func TestCLI_CacheSharedKeyReplacesTheJobName(t *testing.T) {
	j := newJob(t)
	j.cargoProject()
	j.env["CIRCLE_JOB"] = "lint"

	out, err := j.run("--cache-shared-key", "rust", "--cache-key", "extra")
	assert.NilError(t, err, "got: %s", out)
	pending := j.sandbox.Cache.PendingCacheSaves()
	assert.Equal(t, len(pending), 1)
	assert.Assert(t, strings.Contains(pending[0], "-rust-extra-"), "got: %s", pending[0])
	assert.Assert(t, !strings.Contains(pending[0], "lint"), "got: %s", pending[0])
}

func TestCLI_CacheIsKeyedOnTheCompiler(t *testing.T) {
	j := newJob(t)
	j.cargoProject()

	_, err := j.run("--toolchain", "stable")
	assert.NilError(t, err)
	_, err = j.run("--toolchain", "beta")
	assert.NilError(t, err)
	pending := j.sandbox.Cache.PendingCacheSaves()
	assert.Equal(t, len(pending), 2)
	assert.Assert(t, pending[0] != pending[1], "a different compiler must not share build output; got: %v", pending)
}

func TestCLI_CacheSaveIfFalseOnlyRestores(t *testing.T) {
	j := newJob(t)
	j.cargoProject()

	out, err := j.run("--cache-save-if=false")
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, len(j.sandbox.Cache.PendingCacheSaves()), 0)
}

func TestCLI_CacheOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		saved int
	}{
		{"SavesByDefault", nil, 1},
		{"SkipsWhenFalse", []string{"--cache-on-failure=false"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := newJob(t)
			j.cargoProject()
			out, err := j.run(tc.args...)
			assert.NilError(t, err, "got: %s", out)
			j.build()
			j.sandbox.Cache.EndJobAfterFailure(t)
			assert.Equal(t, len(j.sandbox.Cache.SavedKeys()), tc.saved)
		})
	}
}

func TestCLI_NoCache(t *testing.T) {
	j := newJob(t)
	j.cargoProject()

	out, err := j.run("--cache=false")
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, len(j.sandbox.Cache.PendingCacheSaves()), 0)
	assert.Assert(t, !strings.Contains(out, "rustup run"), "got: %s", out)
}

func TestCLI_CacheSkippedWithoutACargoProject(t *testing.T) {
	j := newJob(t)

	out, err := j.run()
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, len(j.sandbox.Cache.PendingCacheSaves()), 0)
	assert.Assert(t, strings.Contains(out, "cache skipped: no Cargo.toml"), "got: %s", out)
}
