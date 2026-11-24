// Package toolchain installs the Rust toolchain a job asks for with rustup,
// following actions-rust-lang/setup-rust-toolchain: what --toolchain names,
// else what a rust-toolchain file names, else stable.
package toolchain

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// DefaultToolchain is installed when neither the flags nor a toolchain file
// name one.
const DefaultToolchain = "stable"

// fileNames are the toolchain files rustup reads, in the order it prefers them.
var fileNames = []string{"rust-toolchain", "rust-toolchain.toml"}

// Runner runs rustup in a directory.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) error
	Output(ctx context.Context, dir string, args ...string) (string, error)
}

// Request is what to install.
type Request struct {
	// Dir is the Rust project's directory: where a toolchain file is read and
	// an override is set.
	Dir string
	// Toolchain is the toolchain to install. Empty defers to a toolchain file
	// in Dir, else DefaultToolchain.
	Toolchain string
	// Targets and Components are added to the toolchain.
	Targets, Components []string
	// Override makes a toolchain named by Toolchain, or DefaultToolchain, the
	// one rustup uses in Dir. A toolchain file already does that.
	Override bool
	// SetDefault makes the toolchain rustup's default, for a rustup installed
	// without one, so cargo works outside Dir too.
	SetDefault bool
}

// File returns the toolchain file rustup would read in dir, or "".
func File(dir string) string {
	for _, name := range fileNames {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// Install installs what req asks for and returns the name of the toolchain
// rustup then uses in req.Dir.
func Install(ctx context.Context, r Runner, req Request) (string, error) {
	if req.Toolchain != "" || File(req.Dir) == "" {
		if err := installNamed(ctx, r, req); err != nil {
			return "", err
		}
		return Active(ctx, r, req.Dir)
	}

	if err := installFromFile(ctx, r, req); err != nil {
		return "", err
	}
	active, err := Active(ctx, r, req.Dir)
	if err != nil {
		return "", err
	}
	if req.SetDefault {
		if err := r.Run(ctx, req.Dir, "default", active); err != nil {
			return "", err
		}
	}
	return active, nil
}

// installFromFile installs the toolchain the file in req.Dir names, with the
// components, targets and profile it lists, then adds req's own. rustup 1.28
// and later install it with a bare toolchain install, and deprecate installing
// it on first use; earlier ones reject a bare toolchain install, and only
// install it on first use, which show active-toolchain is.
func installFromFile(ctx context.Context, r Runner, req Request) error {
	if err := r.Run(ctx, req.Dir, "toolchain", "install"); err != nil {
		if err := r.Run(ctx, req.Dir, "show", "active-toolchain"); err != nil {
			return err
		}
	}
	if len(req.Components) > 0 {
		if err := r.Run(ctx, req.Dir, append([]string{"component", "add"}, req.Components...)...); err != nil {
			return err
		}
	}
	if len(req.Targets) > 0 {
		return r.Run(ctx, req.Dir, append([]string{"target", "add"}, req.Targets...)...)
	}
	return nil
}

// installNamed installs req.Toolchain, or DefaultToolchain, with the minimal
// profile plus the components and targets asked for. A fresh rustup gets it as
// its default before anything asks which toolchain is active, since with no
// override there would be none.
func installNamed(ctx context.Context, r Runner, req Request) error {
	name := cmp.Or(req.Toolchain, DefaultToolchain)
	if err := r.Run(ctx, req.Dir, InstallArgs(req)...); err != nil {
		return err
	}
	if req.SetDefault {
		if err := r.Run(ctx, req.Dir, "default", name); err != nil {
			return err
		}
	}
	if req.Override {
		return r.Run(ctx, req.Dir, "override", "set", name)
	}
	return nil
}

// InstallArgs is the rustup command line installNamed runs. A nightly may
// lack a component on its newest date, so --allow-downgrade lets rustup pick
// the newest nightly that has every one asked for.
func InstallArgs(req Request) []string {
	name := cmp.Or(req.Toolchain, DefaultToolchain)
	args := []string{"toolchain", "install", name}
	for _, t := range req.Targets {
		args = append(args, "--target", t)
	}
	for _, c := range req.Components {
		args = append(args, "--component", c)
	}
	args = append(args, "--profile", "minimal")
	if strings.Contains(name, "nightly") && len(req.Components) > 0 {
		args = append(args, "--allow-downgrade")
	}
	return append(args, "--no-self-update")
}

// Active returns the name of the toolchain rustup uses in dir.
func Active(ctx context.Context, r Runner, dir string) (string, error) {
	out, err := r.Output(ctx, dir, "show", "active-toolchain")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", errors.New("rustup show active-toolchain printed nothing")
	}
	return fields[0], nil
}
