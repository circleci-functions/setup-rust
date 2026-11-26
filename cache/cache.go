// Package cache keeps cargo's downloads, and optionally the build output and
// the binaries cargo install puts in CARGO_HOME/bin, in the CircleCI job cache,
// the way Swatinem/rust-cache does for setup-rust-toolchain.
package cache

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/circleci-functions/toolkit/sdk"
)

// keyVersion changes when what a key holds changes shape, so no restore
// crosses the change.
const keyVersion = "setup-rust-v1"

// depFiles are the files whose contents decide which dependencies a build
// downloads and how it compiles them.
var depFiles = []string{"Cargo.toml", "Cargo.lock", "rust-toolchain", "rust-toolchain.toml"}

// skipDirs are never searched for depFiles.
var skipDirs = []string{"target", ".git", "node_modules"}

// Options is what to cache and how to key it.
type Options struct {
	// CargoHome is the cargo home whose registry, git checkouts and bin
	// directory are cached.
	CargoHome string
	// Dir is the Rust project's directory: where depFiles are found, and
	// whose target directory is cached.
	Dir string
	// Targets includes the target directory.
	Targets bool
	// Bin includes CARGO_HOME/bin and the records cargo install keeps of
	// what it put there.
	Bin bool
	// Key is added to the key.
	Key string
	// Scope is the job name, or a shared key standing in for it, so jobs
	// building differently do not share one cache unless asked to.
	Scope string
	// Rustc identifies the compiler; see RustcHash.
	Rustc string
	// Nonce partitions the cache, so a test run cannot restore an earlier
	// run's.
	Nonce string
}

// Paths returns what to save.
func Paths(o Options) []string {
	paths := Dirs(o)
	if o.Bin {
		paths = append(paths,
			filepath.Join(o.CargoHome, ".crates.toml"),
			filepath.Join(o.CargoHome, ".crates2.json"))
	}
	return paths
}

// Dirs returns the directories among Paths. A build need not create all of
// them, as one with no git dependencies never makes git/db, so Prepare makes
// sure the save finds each one.
func Dirs(o Options) []string {
	dirs := []string{
		filepath.Join(o.CargoHome, "registry", "index"),
		filepath.Join(o.CargoHome, "registry", "cache"),
		filepath.Join(o.CargoHome, "git", "db"),
	}
	if o.Bin {
		dirs = append(dirs, filepath.Join(o.CargoHome, "bin"))
	}
	if o.Targets {
		dirs = append(dirs, TargetDir(o.Dir))
	}
	return dirs
}

// Prepare creates the directories among Paths that do not exist yet.
func Prepare(o Options) error {
	for _, d := range Dirs(o) {
		if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // caches are not secret
			return err
		}
	}
	return nil
}

// TargetDir is where cargo builds the project in dir: $CARGO_TARGET_DIR,
// relative to dir when it is relative, else dir/target.
func TargetDir(dir string) string {
	t := os.Getenv("CARGO_TARGET_DIR")
	switch {
	case t == "":
		return filepath.Join(dir, "target")
	case filepath.IsAbs(t):
		return t
	}
	return filepath.Join(dir, t)
}

// Keys returns the key to save under, and the keys to restore from, best
// first: the exact key, then the newest entry for the same compiler whatever
// the dependencies were.
func Keys(o Options, depsHash string) (save string, restore []string) {
	prefix := strings.Join(nonEmpty(keyVersion+o.Nonce, sdk.OSArch(), o.Scope, o.Key, o.Rustc), "-") + "-"
	save = prefix + depsHash
	return save, []string{save, prefix}
}

// HashDeps hashes the depFiles under dir, and .cargo/config.toml or
// .cargo/config beside any of them. found is false when there is no
// Cargo.toml, so nothing worth caching.
func HashDeps(dir string) (hash string, found bool, err error) {
	var files []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && slices.Contains(skipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		parent := filepath.Base(filepath.Dir(p))
		if slices.Contains(depFiles, name) || (parent == ".cargo" && (name == "config.toml" || name == "config")) {
			files = append(files, p)
		}
		if name == "Cargo.toml" {
			found = true
		}
		return nil
	})
	if err != nil || !found {
		return "", found, err
	}
	slices.Sort(files)

	h := sha256.New()
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return "", false, err
		}
		_, _ = io.WriteString(h, filepath.ToSlash(rel)+"\x00")
		if err := hashFile(h, f); err != nil {
			return "", false, err
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:20], true, nil
}

func hashFile(w io.Writer, path string) error {
	f, err := os.Open(path) //nolint:gosec // a file under the project directory
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// RustcHash shortens the output of rustc -vV to the date and hash of the
// commit the compiler was built from, which is what makes build output
// reusable, as setup-rust-toolchain does for its cache key.
func RustcHash(verbose string) (string, error) {
	var date, hash string
	sc := bufio.NewScanner(strings.NewReader(verbose))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ": ")
		switch {
		case !ok:
		case k == "commit-date":
			date = strings.ReplaceAll(strings.TrimSpace(v), "-", "")
		case k == "commit-hash":
			hash = strings.TrimSpace(v)
		}
	}
	id := date + hash
	if id == "" {
		return "", errors.New("rustc -vV reported no commit-date or commit-hash")
	}
	return id[:min(len(id), 12)], nil
}

func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
