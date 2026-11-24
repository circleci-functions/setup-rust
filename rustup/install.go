package rustup

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultUpdateRoot is where rustup-init is published. $RUSTUP_UPDATE_ROOT
// overrides it, as it does for rustup's own installer script.
const DefaultUpdateRoot = "https://static.rust-lang.org/rustup"

const downloadAttempts = 3

// retryDelay is the wait before the first retry; each later one waits longer.
var retryDelay = 2 * time.Second

// InstallOptions configures Install.
type InstallOptions struct {
	// Triple is the host triple to install for; see HostTriple.
	Triple string
	// UpdateRoot is the base URL rustup-init is fetched from. Empty is
	// $RUSTUP_UPDATE_ROOT, else DefaultUpdateRoot.
	UpdateRoot string
	// HTTP fetches rustup-init. Nil is http.DefaultClient.
	HTTP *http.Client
	// Stdout and Stderr receive rustup-init's output.
	Stdout, Stderr io.Writer
}

// Install fetches rustup-init, checks it against its published SHA-256 and
// runs it, so rustup lands in CARGO_HOME/bin without a default toolchain and
// without touching any shell profile. It returns the path of the rustup it
// installed.
func Install(ctx context.Context, opts InstallOptions) (string, error) {
	root := strings.TrimSuffix(cmp.Or(opts.UpdateRoot, os.Getenv("RUSTUP_UPDATE_ROOT"), DefaultUpdateRoot), "/")
	client := cmp.Or(opts.HTTP, http.DefaultClient)
	url := fmt.Sprintf("%s/dist/%s/%s", root, opts.Triple, exeName("rustup-init"))

	dir, err := os.MkdirTemp("", "setup-rust-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	initPath, err := fetchVerified(ctx, client, url, dir)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, initPath, "-y", "--no-modify-path", "--default-toolchain", "none") //nolint:gosec // rustup-init, checked against its published SHA-256
	cmd.Env = append(os.Environ(), "RUSTUP_INIT_SKIP_PATH_CHECK=yes")
	cmd.Stdout, cmd.Stderr = opts.Stdout, opts.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rustup-init: %w", err)
	}

	home, err := CargoHome()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(home, "bin", exeName("rustup"))
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("rustup-init finished, but left no rustup: %w", err)
	}
	return bin, nil
}

// fetchVerified downloads url into dir, and fails unless its SHA-256 is the
// one published beside it at url.sha256.
func fetchVerified(ctx context.Context, client *http.Client, url, dir string) (string, error) {
	sum, err := get(ctx, client, url+".sha256")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 {
		return "", fmt.Errorf("download %s.sha256: empty", url)
	}
	want := strings.ToLower(fields[0])

	body, err := get(ctx, client, url)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(body)
	if got := hex.EncodeToString(h[:]); got != want {
		return "", fmt.Errorf("download %s: SHA-256 is %s, want %s", url, got, want)
	}

	p := filepath.Join(dir, exeName("rustup-init"))
	if err := os.WriteFile(p, body, 0o755); err != nil { //nolint:gosec // must be executable
		return "", err
	}
	return p, nil
}

var errNotFound = errors.New("not found")

// get fetches url, retrying failures other than a 404.
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	var err error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		var body []byte
		if body, err = getOnce(ctx, client, url); err == nil {
			return body, nil
		}
		if errors.Is(err, errNotFound) {
			return nil, err
		}
		if attempt < downloadAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * retryDelay):
			}
		}
	}
	return nil, err
}

func getOnce(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // the rustup update root the job configures
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req) //nolint:gosec // the rustup update root the job configures
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("download %s: %w", url, errNotFound)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	return buf.Bytes(), nil
}

// CargoHome returns the directory rustup installs its proxies under:
// $CARGO_HOME, else ~/.cargo.
func CargoHome() (string, error) {
	if dir := os.Getenv("CARGO_HOME"); dir != "" {
		return filepath.Abs(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cargo"), nil
}

// Find returns the rustup to use without installing one: the one PATH
// resolves, else one an earlier install left in CARGO_HOME/bin. ok is false
// when there is neither.
func Find() (bin string, ok bool) {
	if p, err := exec.LookPath("rustup"); err == nil {
		return p, true
	}
	home, err := CargoHome()
	if err != nil {
		return "", false
	}
	p := filepath.Join(home, "bin", exeName("rustup"))
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p, true
	}
	return "", false
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
