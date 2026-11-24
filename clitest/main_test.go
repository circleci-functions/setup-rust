// Package clitest_test runs the setup-rust binary end to end against fakes: a
// fake static.rust-lang.org serving a fake rustup-init, which installs a fake
// rustup and proxies. Hermetic, so it runs under task test. Real rustup is
// covered by acceptance/.
package clitest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"

	"github.com/circleci-functions/setup-rust/rustup"
)

var binaryPath, fakeRustupPath string

func TestMain(m *testing.M) {
	status, err := runTests(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(status)
}

func runTests(m *testing.M) (int, error) {
	p := compiler.NewParallel(2)
	defer p.Cleanup()
	p.Add(compiler.Work{Result: &binaryPath, Name: "setup-rust", Target: "..", Source: "."})
	p.Add(compiler.Work{Result: &fakeRustupPath, Name: "fakerustup", Target: "../internal/testing/fakerustup", Source: "."})
	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// job is one sandboxed run environment: a home directory, a PATH holding only
// the sandbox's bin directory, a working directory and a fake rustup server.
type job struct {
	t       *testing.T
	sandbox *sdktest.Sandbox
	dist    *dist
	wd      string
	log     string
	env     map[string]string
}

func newJob(t *testing.T) *job {
	t.Helper()
	s := sdktest.NewSandbox(t)
	j := &job{t: t, sandbox: s, dist: newDist(t), wd: t.TempDir(), log: filepath.Join(t.TempDir(), "calls.log")}
	j.env = map[string]string{
		"PATH":               systemPath(s.BinDir),
		"USERPROFILE":        s.Home,
		"CARGO_HOME":         "",
		"RUSTUP_HOME":        "",
		"RUSTUP_TOOLCHAIN":   "",
		"RUSTUP_UPDATE_ROOT": j.dist.URL,
		"FAKERUSTUP_LOG":     j.log,
	}
	return j
}

// run runs setup-rust in the job's working directory, linking into the
// sandbox's bin directory.
func (j *job) run(args ...string) (string, error) {
	j.t.Helper()
	env := j.sandbox.Env()
	maps.Copy(env, j.env)
	args = append(args, "--bin-dir", j.sandbox.BinDir)
	r := runner.New(j.t, binaryPath, runner.WithSandboxEnv(env), runner.WithCmdDir(j.wd))
	_, err := r.Run(args...)
	return r.StdoutString() + r.StderrString(), err
}

// later runs name from the sandbox's bin directory in dir, as a later step
// would.
func (j *job) later(dir, name string, args ...string) string {
	j.t.Helper()
	cmd := exec.CommandContext(j.t.Context(), filepath.Join(j.sandbox.BinDir, name), args...)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(j.t.Context(), "cmd", append([]string{"/c", filepath.Join(j.sandbox.BinDir, name+".bat")}, args...)...)
	}
	cmd.Dir = dir
	env := j.sandbox.Env()
	maps.Copy(env, j.env)
	cmd.Env = j.sandbox.CmdEnv(os.Environ())
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	assert.NilError(j.t, err, "%s: %s", name, out)
	return strings.TrimSpace(string(out))
}

// calls returns what the fake rustup-init, rustup and proxies were run with.
func (j *job) calls() []string {
	var out []string
	for _, c := range j.callsIn() {
		out = append(out, c[0])
	}
	return out
}

// callsIn returns each call with the directory it ran in.
func (j *job) callsIn() [][2]string {
	b, err := os.ReadFile(j.log)
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(j.t, err)
	var out [][2]string
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		call, dir, _ := strings.Cut(line, "\t")
		out = append(out, [2]string{call, dir})
	}
	return out
}

func (j *job) write(name, content string) {
	j.t.Helper()
	p := filepath.Join(j.wd, filepath.FromSlash(name))
	assert.NilError(j.t, os.MkdirAll(filepath.Dir(p), 0o750))
	assert.NilError(j.t, os.WriteFile(p, []byte(content), 0o600))
}

// putRustupOnPath installs the fake rustup and cargo in a directory on PATH,
// as an image that ships Rust would, with toolchain as its default.
func (j *job) putRustupOnPath(toolchain string) string {
	j.t.Helper()
	dir := filepath.Join(j.t.TempDir(), "cargo", "bin")
	for _, name := range []string{"rustup", "cargo"} {
		copyExe(j.t, fakeRustupPath, filepath.Join(dir, exe(name)))
	}
	rustupHome := filepath.Join(j.sandbox.Home, ".rustup")
	assert.NilError(j.t, os.MkdirAll(filepath.Join(rustupHome, "toolchains", toolchain), 0o750))
	assert.NilError(j.t, os.WriteFile(filepath.Join(rustupHome, "default"), []byte(toolchain), 0o600))
	j.env["PATH"] = dir + string(os.PathListSeparator) + j.env["PATH"]
	return dir
}

func systemPath(dir string) string {
	if runtime.GOOS == "windows" {
		return dir + string(os.PathListSeparator) + filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	return dir
}

func copyExe(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	assert.NilError(t, err)
	assert.NilError(t, os.MkdirAll(filepath.Dir(dst), 0o750))
	assert.NilError(t, os.WriteFile(dst, b, 0o755)) //nolint:gosec // a test executable
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// dist is a fake static.rust-lang.org/rustup, serving the fake as this
// machine's rustup-init.
type dist struct {
	*httptest.Server
	requests atomic.Int32
	checksum string
}

func newDist(t *testing.T) *dist {
	t.Helper()
	triple, err := rustup.HostTriple()
	assert.NilError(t, err)
	body, err := os.ReadFile(fakeRustupPath)
	assert.NilError(t, err)
	h := sha256.Sum256(body)
	d := &dist{checksum: hex.EncodeToString(h[:])}
	path := "/dist/" + triple + "/" + exe("rustup-init")
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.requests.Add(1)
		switch r.URL.Path {
		case path:
			_, _ = w.Write(body)
		case path + ".sha256":
			_, _ = fmt.Fprintf(w, "%s *./rustup-init\n", d.checksum)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(d.Close)
	return d
}
