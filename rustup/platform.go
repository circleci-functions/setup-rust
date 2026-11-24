// Package rustup installs rustup and runs it.
package rustup

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// muslLoader matches the dynamic loader a musl distribution such as Alpine
// ships, which glibc ones do not.
var muslLoader = "/lib/ld-musl-*.so.1"

// HostTriple returns the target triple rustup-init is published under for the
// machine this runs on, such as x86_64-unknown-linux-gnu.
func HostTriple() (string, error) {
	musl := false
	if runtime.GOOS == "linux" {
		matches, _ := filepath.Glob(muslLoader)
		musl = len(matches) > 0
	}
	return hostTriple(runtime.GOOS, runtime.GOARCH, musl)
}

func hostTriple(goos, goarch string, musl bool) (string, error) {
	arch, ok := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	if ok {
		switch goos {
		case "linux":
			if musl {
				return arch + "-unknown-linux-musl", nil
			}
			return arch + "-unknown-linux-gnu", nil
		case "darwin":
			return arch + "-apple-darwin", nil
		case "windows":
			return arch + "-pc-windows-msvc", nil
		}
	}
	return "", fmt.Errorf("setup-rust does not support %s/%s", goos, goarch)
}
