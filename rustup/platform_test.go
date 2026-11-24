package rustup

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestHostTriple(t *testing.T) {
	for _, tc := range []struct {
		goos, goarch string
		musl         bool
		want         string
	}{
		{"linux", "amd64", false, "x86_64-unknown-linux-gnu"},
		{"linux", "arm64", false, "aarch64-unknown-linux-gnu"},
		{"linux", "amd64", true, "x86_64-unknown-linux-musl"},
		{"linux", "arm64", true, "aarch64-unknown-linux-musl"},
		{"darwin", "amd64", false, "x86_64-apple-darwin"},
		{"darwin", "arm64", false, "aarch64-apple-darwin"},
		{"windows", "amd64", false, "x86_64-pc-windows-msvc"},
		{"windows", "arm64", false, "aarch64-pc-windows-msvc"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			got, err := hostTriple(tc.goos, tc.goarch, tc.musl)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestHostTriple_Unsupported(t *testing.T) {
	for _, p := range [][2]string{{"linux", "386"}, {"freebsd", "amd64"}, {"windows", "arm"}} {
		_, err := hostTriple(p[0], p[1], false)
		assert.ErrorContains(t, err, "does not support "+p[0]+"/"+p[1])
	}
}
