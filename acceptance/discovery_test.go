//go:build acceptance

package acceptance_test

import (
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// TestAcceptance_InstallsOntoTheRealPath is the one test that lets setup-rust
// install rustup and choose where on the executor's real PATH the proxies go,
// then checks a later step finds them by name. It changes the machine, so it
// only runs as a step in a CircleCI job, where the smoke jobs run it on each
// platform, or where SETUP_RUST_DISPOSABLE says the machine is a throwaway
// container, as for task smoke-local.
func TestAcceptance_InstallsOntoTheRealPath(t *testing.T) {
	if os.Getenv("SETUP_RUST_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars(), runner.WithCmdDir(t.TempDir()))
	_, err := r.Run("--toolchain", acceptanceToolchain(), "--components", "rustfmt,clippy")
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "got: %s", out)
	t.Logf("setup-rust: %s", out)

	for _, args := range [][]string{{"cargo", "--version"}, {"rustc", "--version"}, {"cargo", "fmt", "--version"}, {"cargo", "clippy", "--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got, err := sdktest.LaterStep(t, args[0], args[1:]...)
			assert.NilError(t, err, "a later step could not run %s: %s", strings.Join(args, " "), got)
		})
	}
}
