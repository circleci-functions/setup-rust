//go:build acceptance

package acceptance_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// acceptanceToolchain returns the toolchain to install:
// SETUP_RUST_ACCEPTANCE_TOOLCHAIN in CI, from the pipeline's parameter, else
// stable.
func acceptanceToolchain() string {
	if v := os.Getenv("SETUP_RUST_ACCEPTANCE_TOOLCHAIN"); v != "" {
		return v
	}
	return "stable"
}

// job is a sandboxed run: rustup and its toolchains go under a temporary
// home, and the proxies are linked into the sandbox's bin directory, which is
// all of PATH apart from the system directories.
type job struct {
	t       *testing.T
	sandbox *sdktest.Sandbox
	wd      string
	env     map[string]string
}

func newJob(t *testing.T, fixture string) *job {
	t.Helper()
	s := sdktest.NewSandbox(t)
	j := &job{t: t, sandbox: s, wd: t.TempDir()}
	if fixture != "" {
		assert.NilError(t, os.CopyFS(j.wd, os.DirFS(filepath.Join("testdata", fixture))))
	}
	j.env = s.Env()
	maps.Copy(j.env, map[string]string{
		"PATH":             systemPath(s.BinDir),
		"USERPROFILE":      s.Home,
		"CARGO_HOME":       "",
		"RUSTUP_HOME":      "",
		"RUSTUP_TOOLCHAIN": "",
	})
	return j
}

func (j *job) run(args ...string) string {
	j.t.Helper()
	r := runner.New(j.t, binaryPath, runner.WithSandboxEnv(j.env), runner.WithCmdDir(j.wd))
	_, err := r.Run(append(args, "--bin-dir", j.sandbox.BinDir)...)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(j.t, err, "got: %s", out)
	return out
}

// later runs a command line in the working directory the way a later step
// would: a fresh shell, with PATH as the only way to find cargo.
func (j *job) later(command string) (string, error) {
	j.t.Helper()
	cmd := exec.CommandContext(j.t.Context(), "sh", "-c", command)
	cmd.Dir = j.wd
	cmd.Env = os.Environ()
	for k, v := range j.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (j *job) mustLater(command string) string {
	j.t.Helper()
	out, err := j.later(command)
	assert.NilError(j.t, err, "%s: %s", command, out)
	return out
}

func systemPath(dir string) string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		return strings.Join([]string{dir, filepath.Join(root, "System32"), filepath.Dir(must(exec.LookPath("sh")))}, string(os.PathListSeparator))
	}
	return dir + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin"
}

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

func TestAcceptance_InstallsAToolchainWithComponentsAndTargets(t *testing.T) {
	j := newJob(t, "hello")
	toolchain := acceptanceToolchain()

	out := j.run("--toolchain", toolchain, "--components", "rustfmt,clippy", "--target", "wasm32-unknown-unknown")
	t.Logf("setup-rust: %s", out)

	t.Run("ToolchainResolvesFromPath", func(t *testing.T) {
		got := j.mustLater("rustc --version")
		if toolchain != "stable" {
			assert.Assert(t, strings.Contains(got, "rustc "+toolchain), "got: %s", got)
		}
	})
	t.Run("ComponentsAreInstalled", func(t *testing.T) {
		j.mustLater("cargo fmt --version")
		j.mustLater("cargo clippy --version")
	})
	t.Run("TargetIsInstalled", func(t *testing.T) {
		assert.Assert(t, strings.Contains(j.mustLater("rustup target list --installed"), "wasm32-unknown-unknown"))
	})
	t.Run("BuildsAndRunsACrate", func(t *testing.T) {
		assert.Equal(t, j.mustLater("cargo run --quiet"), "hello from setup-rust")
	})
}

func TestAcceptance_InstallsWhatTheToolchainFileNames(t *testing.T) {
	j := newJob(t, "pinned")

	out := j.run()
	t.Logf("setup-rust: %s", out)

	assert.Assert(t, strings.Contains(j.mustLater("rustc --version"), "rustc 1.99.1"))
	t.Run("ComponentsTheFileListsAreInstalled", func(t *testing.T) {
		j.mustLater("cargo fmt --version")
	})
	t.Run("TheFilesMinimalProfileLeavesTheRestOut", func(t *testing.T) {
		got, err := j.later("cargo clippy --version")
		assert.Assert(t, err != nil, "clippy is not in the file, so must not be installed; got: %s", got)
	})
}
