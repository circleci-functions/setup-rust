package rustup

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"gotest.tools/v3/assert"
)

func serve(t *testing.T, files map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestFetchVerified(t *testing.T) {
	srv, _ := serve(t, map[string]string{
		"/rustup-init":        "binary",
		"/rustup-init.sha256": sha("binary") + " *./rustup-init\n",
	})
	p, err := fetchVerified(t.Context(), srv.Client(), srv.URL+"/rustup-init", t.TempDir())
	assert.NilError(t, err)

	b, err := os.ReadFile(p)
	assert.NilError(t, err)
	assert.Equal(t, string(b), "binary")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(p)
		assert.NilError(t, err)
		assert.Equal(t, info.Mode().Perm()&0o100, os.FileMode(0o100), "rustup-init must be executable")
	}
}

func TestFetchVerified_AcceptsABareChecksum(t *testing.T) {
	srv, _ := serve(t, map[string]string{
		"/rustup-init":        "binary",
		"/rustup-init.sha256": sha("binary"),
	})
	_, err := fetchVerified(t.Context(), srv.Client(), srv.URL+"/rustup-init", t.TempDir())
	assert.NilError(t, err)
}

func TestFetchVerified_RejectsAChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	srv, _ := serve(t, map[string]string{
		"/rustup-init":        "tampered",
		"/rustup-init.sha256": sha("binary") + " *./rustup-init\n",
	})
	_, err := fetchVerified(t.Context(), srv.Client(), srv.URL+"/rustup-init", dir)
	assert.ErrorContains(t, err, "SHA-256 is "+sha("tampered")+", want "+sha("binary"))

	entries, err := os.ReadDir(dir)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 0, "a binary that fails its checksum must not be written")
}

func TestFetchVerified_DoesNotRetryANotFound(t *testing.T) {
	srv, requests := serve(t, map[string]string{})
	_, err := fetchVerified(t.Context(), srv.Client(), srv.URL+"/rustup-init", t.TempDir())
	assert.ErrorIs(t, err, errNotFound)
	assert.Equal(t, requests.Load(), int32(1))
}

func TestCargoHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	t.Setenv("CARGO_HOME", "")
	got, err := CargoHome()
	assert.NilError(t, err)
	assert.Equal(t, got, filepath.Join(home, ".cargo"))

	custom := filepath.Join(t.TempDir(), "cargo")
	t.Setenv("CARGO_HOME", custom)
	got, err = CargoHome()
	assert.NilError(t, err)
	assert.Equal(t, got, custom)
}

func TestFind(t *testing.T) {
	cargoHome := t.TempDir()
	t.Setenv("CARGO_HOME", cargoHome)
	t.Setenv("PATH", t.TempDir())

	_, ok := Find()
	assert.Assert(t, !ok, "no rustup on PATH or in CARGO_HOME")

	bin := filepath.Join(cargoHome, "bin", exeName("rustup"))
	assert.NilError(t, os.MkdirAll(filepath.Dir(bin), 0o750))
	assert.NilError(t, os.WriteFile(bin, nil, 0o755)) //nolint:gosec // a test executable
	got, ok := Find()
	assert.Assert(t, ok)
	assert.Equal(t, got, bin)
}

func TestFind_PrefersTheRustupOnPath(t *testing.T) {
	t.Setenv("CARGO_HOME", t.TempDir())
	onPath := t.TempDir()
	t.Setenv("PATH", onPath)
	bin := filepath.Join(onPath, exeName("rustup"))
	assert.NilError(t, os.WriteFile(bin, nil, 0o755)) //nolint:gosec // a test executable

	got, ok := Find()
	assert.Assert(t, ok)
	assert.Equal(t, got, bin)
}
