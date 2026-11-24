package rustpath

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		assert.NilError(t, os.WriteFile(filepath.Join(dir, exeName(name)), nil, 0o755)) //nolint:gosec // a test executable
	}
}

func TestPresent(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "rustfmt", "rustup", "cargo", "cargo-nextest")
	assert.NilError(t, os.Mkdir(filepath.Join(dir, exeName("rustc")), 0o750))

	assert.DeepEqual(t, Present(dir), []string{"cargo", "rustup", "rustfmt"})
}

func TestLink_IntoBinDir(t *testing.T) {
	cargoBin, binDir := t.TempDir(), t.TempDir()
	touch(t, cargoBin, "rustup", "cargo", "rustc")

	res, err := Link(Options{CargoBin: cargoBin, BinDir: binDir})
	assert.NilError(t, err)
	assert.Equal(t, res.Dir, binDir)
	for _, name := range []string{"cargo", "rustup", "rustc"} {
		_, err := os.Lstat(filepath.Join(binDir, name))
		assert.NilError(t, err, "expected %s to be linked", name)
	}
}

func TestLink_FailsWithoutCargo(t *testing.T) {
	cargoBin := t.TempDir()
	touch(t, cargoBin, "rustup")

	_, err := Link(Options{CargoBin: cargoBin, BinDir: t.TempDir()})
	assert.ErrorContains(t, err, "no cargo in "+cargoBin)
}
