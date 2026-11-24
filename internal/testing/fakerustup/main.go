// Command fakerustup stands in for rustup-init, rustup and rustup's proxies in
// tests, telling them apart by the name it runs as. It records every call in
// $FAKERUSTUP_LOG, keeps the toolchains it "installs" as directories under
// RUSTUP_HOME, and resolves the active toolchain the way rustup does: a
// directory override, else a toolchain file, else the default.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// proxies are what rustup-init installs beside rustup.
var proxies = []string{"cargo", "rustc", "rustdoc", "rustfmt", "cargo-fmt", "cargo-clippy", "clippy-driver"}

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	record(name)
	switch name {
	case "rustup-init":
		initRustup()
	case "rustup":
		os.Exit(rustup(os.Args[1:]))
	default:
		os.Exit(proxy(name))
	}
}

func initRustup() {
	self, err := os.Executable()
	if err != nil {
		fail(err)
	}
	b, err := os.ReadFile(self) //nolint:gosec // this executable
	if err != nil {
		fail(err)
	}
	bin := filepath.Join(home("CARGO_HOME", ".cargo"), "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		fail(err)
	}
	for _, name := range append([]string{"rustup"}, proxies...) {
		if err := os.WriteFile(filepath.Join(bin, exe(name)), b, 0o755); err != nil { //nolint:gosec // must be executable
			fail(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(rustupHome(), "toolchains"), 0o750); err != nil {
		fail(err)
	}
}

func rustup(args []string) int {
	wd, _ := os.Getwd()
	switch {
	case len(args) > 0 && args[0] == "--version":
		fmt.Println("rustup 1.28.2 (fake)")
	case len(args) > 1 && args[0] == "toolchain" && args[1] == "install":
		return toolchainInstall(wd, args[2:])
	case len(args) > 1 && args[0] == "show" && args[1] == "active-toolchain":
		name, ok := active(wd)
		if !ok {
			fmt.Fprintln(os.Stderr, "error: no active toolchain")
			return 1
		}
		fmt.Println(name + " (fake)")
	case len(args) > 2 && args[0] == "override" && args[1] == "set":
		appendLine(filepath.Join(rustupHome(), "overrides"), wd+"\t"+args[2])
	case len(args) > 1 && args[0] == "default":
		writeFile(filepath.Join(rustupHome(), "default"), args[1])
	}
	return 0
}

// toolchainInstall installs the toolchain args name, else the one the
// toolchain file in wd names.
func toolchainInstall(wd string, args []string) int {
	name := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
	} else if name = fileChannel(wd); name == "" {
		fmt.Fprintln(os.Stderr, "error: no toolchain file")
		return 1
	}
	install(name)
	return 0
}

func proxy(name string) int {
	wd, _ := os.Getwd()
	tc, ok := active(wd)
	if !ok {
		fmt.Fprintln(os.Stderr, "error: rustup could not choose a version of "+name+" to run")
		return 1
	}
	fmt.Printf("%s 1.0.0-fake (%s)\n", name, tc)
	return 0
}

// active resolves the toolchain rustup would use in dir, reporting false when
// it is not installed.
func active(dir string) (string, bool) {
	name := ""
	if b, err := os.ReadFile(filepath.Join(rustupHome(), "overrides")); err == nil { //nolint:gosec // the fake's own state
		for line := range strings.SplitSeq(string(b), "\n") {
			if d, tc, ok := strings.Cut(line, "\t"); ok && d == dir {
				name = tc
			}
		}
	}
	if name == "" {
		name = fileChannel(dir)
	}
	if name == "" {
		b, _ := os.ReadFile(filepath.Join(rustupHome(), "default")) //nolint:gosec // the fake's own state
		name = strings.TrimSpace(string(b))
	}
	if name == "" {
		return "", false
	}
	_, err := os.Stat(filepath.Join(rustupHome(), "toolchains", name)) //nolint:gosec // the fake's own state
	return name, err == nil
}

// fileChannel returns the channel the toolchain file in dir names, or "".
func fileChannel(dir string) string {
	for _, f := range []string{"rust-toolchain", "rust-toolchain.toml"} {
		b, err := os.ReadFile(filepath.Join(dir, f)) //nolint:gosec // a test fixture
		if err != nil {
			continue
		}
		s := string(b)
		if !strings.Contains(s, "[toolchain]") {
			return strings.TrimSpace(s)
		}
		sc := bufio.NewScanner(strings.NewReader(s))
		for sc.Scan() {
			if k, v, ok := strings.Cut(sc.Text(), "="); ok && strings.TrimSpace(k) == "channel" {
				return strings.Trim(strings.TrimSpace(v), `"`)
			}
		}
	}
	return ""
}

func install(name string) {
	if err := os.MkdirAll(filepath.Join(rustupHome(), "toolchains", name), 0o750); err != nil { //nolint:gosec // the fake's own state
		fail(err)
	}
}

func record(name string) {
	log := os.Getenv("FAKERUSTUP_LOG")
	if log == "" {
		return
	}
	wd, _ := os.Getwd()
	appendLine(log, strings.TrimSpace(name+" "+strings.Join(os.Args[1:], " "))+"\t"+wd)
}

func appendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // a test file
	if err != nil {
		fail(err)
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // the fake's own state
		fail(err)
	}
}

func rustupHome() string {
	return home("RUSTUP_HOME", ".rustup")
}

func home(env, dir string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	return filepath.Join(h, dir)
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fakerustup:", err)
	os.Exit(2)
}
