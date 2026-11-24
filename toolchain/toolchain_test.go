package toolchain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// fakeRunner records the rustup commands run, failing those listed in fail.
type fakeRunner struct {
	calls  []string
	fail   map[string]bool
	active string
}

func (f *fakeRunner) Run(_ context.Context, _ string, args ...string) error {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.fail[call] {
		return errors.New("failed: " + call)
	}
	return nil
}

func (f *fakeRunner) Output(_ context.Context, _ string, args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " ")+" (output)")
	return f.active + " (overridden by '/project/rust-toolchain.toml')\n", nil
}

func TestInstall_StableWhenNothingIsAsked(t *testing.T) {
	r := &fakeRunner{active: "stable-x86_64-unknown-linux-gnu"}
	active, err := Install(t.Context(), r, Request{Dir: t.TempDir(), Override: true})
	assert.NilError(t, err)
	assert.Equal(t, active, "stable-x86_64-unknown-linux-gnu")
	assert.DeepEqual(t, r.calls, []string{
		"toolchain install stable --profile minimal --no-self-update",
		"override set stable",
		"show active-toolchain (output)",
	})
}

func TestInstall_TheNamedToolchainWithTargetsAndComponents(t *testing.T) {
	r := &fakeRunner{active: "1.83-x86_64-unknown-linux-gnu"}
	_, err := Install(t.Context(), r, Request{
		Dir:        t.TempDir(),
		Toolchain:  "1.83",
		Targets:    []string{"wasm32-unknown-unknown"},
		Components: []string{"rustfmt", "clippy"},
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, r.calls, []string{
		"toolchain install 1.83 --target wasm32-unknown-unknown --component rustfmt --component clippy --profile minimal --no-self-update",
		"show active-toolchain (output)",
	})
}

func TestInstall_SetsTheDefaultForAFreshRustup(t *testing.T) {
	r := &fakeRunner{active: "stable-x86_64-unknown-linux-gnu"}
	_, err := Install(t.Context(), r, Request{Dir: t.TempDir(), SetDefault: true})
	assert.NilError(t, err)
	assert.DeepEqual(t, r.calls, []string{
		"toolchain install stable --profile minimal --no-self-update",
		"default stable",
		"show active-toolchain (output)",
	})
}

func TestInstall_SetsTheDefaultForAFreshRustupFromAToolchainFile(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "rust-toolchain"), []byte("1.83\n"), 0o600))
	r := &fakeRunner{active: "1.83-x86_64-unknown-linux-gnu"}
	_, err := Install(t.Context(), r, Request{Dir: dir, SetDefault: true})
	assert.NilError(t, err)
	assert.Equal(t, r.calls[len(r.calls)-1], "default 1.83-x86_64-unknown-linux-gnu")
}

func TestInstall_FromAToolchainFile(t *testing.T) {
	for _, name := range fileNames {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			assert.NilError(t, os.WriteFile(filepath.Join(dir, name), []byte("[toolchain]\nchannel = \"1.83\"\n"), 0o600))
			r := &fakeRunner{active: "1.83-x86_64-unknown-linux-gnu"}

			_, err := Install(t.Context(), r, Request{Dir: dir, Override: true, Components: []string{"rust-src"}, Targets: []string{"wasm32-wasip1"}})
			assert.NilError(t, err)
			assert.DeepEqual(t, r.calls, []string{
				"toolchain install",
				"component add rust-src",
				"target add wasm32-wasip1",
				"show active-toolchain (output)",
			})
		})
	}
}

func TestInstall_FromAToolchainFileFallsBackForAnOldRustup(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "rust-toolchain.toml"), []byte("[toolchain]\nchannel = \"1.83\"\n"), 0o600))
	r := &fakeRunner{active: "1.83", fail: map[string]bool{"toolchain install": true}}

	_, err := Install(t.Context(), r, Request{Dir: dir})
	assert.NilError(t, err)
	assert.DeepEqual(t, r.calls, []string{
		"toolchain install",
		"show active-toolchain",
		"show active-toolchain (output)",
	})
}

func TestInstall_TheFlagWinsOverAToolchainFile(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "rust-toolchain.toml"), []byte("[toolchain]\nchannel = \"1.83\"\n"), 0o600))
	r := &fakeRunner{active: "beta"}

	_, err := Install(t.Context(), r, Request{Dir: dir, Toolchain: "beta", Override: true})
	assert.NilError(t, err)
	assert.Equal(t, r.calls[0], "toolchain install beta --profile minimal --no-self-update")
	assert.Equal(t, r.calls[1], "override set beta")
}

func TestInstall_StopsAtAFailedInstall(t *testing.T) {
	r := &fakeRunner{fail: map[string]bool{"toolchain install stable --profile minimal --no-self-update": true}}
	_, err := Install(t.Context(), r, Request{Dir: t.TempDir(), Override: true})
	assert.ErrorContains(t, err, "failed: toolchain install stable")
	assert.Equal(t, len(r.calls), 1)
}

func TestInstallArgs_AllowsANightlyToDowngradeForItsComponents(t *testing.T) {
	assert.DeepEqual(t, InstallArgs(Request{Toolchain: "nightly-2026-01-01", Components: []string{"miri"}}), []string{
		"toolchain", "install", "nightly-2026-01-01", "--component", "miri", "--profile", "minimal", "--allow-downgrade", "--no-self-update",
	})
	assert.DeepEqual(t, InstallArgs(Request{Toolchain: "nightly"}), []string{
		"toolchain", "install", "nightly", "--profile", "minimal", "--no-self-update",
	})
}

func TestFile(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, File(dir), "")

	assert.NilError(t, os.WriteFile(filepath.Join(dir, "rust-toolchain.toml"), nil, 0o600))
	assert.Equal(t, File(dir), filepath.Join(dir, "rust-toolchain.toml"))

	assert.NilError(t, os.WriteFile(filepath.Join(dir, "rust-toolchain"), nil, 0o600))
	assert.Equal(t, File(dir), filepath.Join(dir, "rust-toolchain"), "rustup prefers rust-toolchain")
}
