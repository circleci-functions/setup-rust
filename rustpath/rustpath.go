// Package rustpath puts rustup's proxies on PATH for the rest of the job, so
// later steps run cargo and rustc in whatever shell they use, with nothing to
// source.
package rustpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/circleci-functions/toolkit/pathlink"
)

// proxies are the names rustup answers to, cargo first since it is the one
// verified. A toolchain without a component still gets its proxy, which says
// how to add it when run.
var proxies = []string{
	"cargo", "rustup", "rustc", "rustdoc", "rustfmt", "cargo-fmt", "cargo-clippy", "clippy-driver",
	"cargo-miri", "rust-analyzer", "rust-gdb", "rust-gdbgui", "rust-lldb", "rls",
}

// Options configures Link.
type Options struct {
	// CargoBin is the directory holding rustup and its proxies.
	CargoBin string
	// BinDir links into exactly this directory and skips discovery. Tests set
	// it, so they cannot write to the real PATH.
	BinDir string
	// Path is the search path to use. Empty means this process's PATH.
	Path string
}

// Link makes the proxies in opts.CargoBin resolvable by name, and reports
// where they went. A CargoBin already on PATH is left as it is.
func Link(opts Options) (pathlink.Result, error) {
	names := Present(opts.CargoBin)
	if len(names) == 0 || names[0] != "cargo" {
		return pathlink.Result{}, fmt.Errorf("no cargo in %s", opts.CargoBin)
	}
	return pathlink.Ensure(pathlink.Options{
		Names:     names,
		FromDir:   opts.CargoBin,
		Preferred: pathlink.UserBinDirs(),
		Dir:       opts.BinDir,
		Path:      opts.Path,
		Verify:    verifyCargo,
	})
}

// Present returns the proxies dir holds, in the order of proxies.
func Present(dir string) []string {
	var out []string
	for _, name := range proxies {
		if info, err := os.Stat(filepath.Join(dir, exeName(name))); err == nil && !info.IsDir() {
			out = append(out, name)
		}
	}
	return out
}

// verifyCargo fails unless bin runs as cargo, which takes a rustup proxy and
// an installed toolchain behind it.
func verifyCargo(bin string) error {
	out, err := pathlink.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return errors.Join(fmt.Errorf("%s --version: %s", bin, out), err)
	}
	return nil
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
