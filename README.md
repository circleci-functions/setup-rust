# setup-rust

Installs a Rust toolchain with rustup, plus any components and targets you
ask for, and puts `cargo`, `rustc` and the other rustup proxies on `PATH` for
the rest of the job.

Its flags follow the inputs of the
[setup-rust-toolchain](https://github.com/actions-rust-lang/setup-rust-toolchain)
GitHub Action.

```yaml
version: 2.1

functions:
  setup-rust: github.com/circleci-functions/setup-rust@v0.1.0

jobs:
  check:
    docker:
      - image: cimg/base:stable
    steps:
      - checkout
      - setup-rust:
          with:
            components: rustfmt,clippy
      - run: cargo fmt --check
      - run: cargo clippy --locked -- -D warnings
      - run: cargo test --locked
```

Later steps pick up `cargo` and `rustc` automatically, in whatever shell they
run. There is nothing to source or export yourself.

`circleci function add setup-rust` writes the `functions:` entry for you,
pinned to the latest release. Each [flag](#flags) is a `with:` key, named
without its leading `--`.

Where the `functions:` block isn't available, as on CircleCI Server, call
setup-rust by its full name with `circleci run` and pass the flags after `--`:

```yaml
- run: circleci run github.com/circleci-functions/setup-rust@v0.1.0 -- --components rustfmt,clippy
```

## Choosing a toolchain

Without `toolchain`, setup-rust installs the toolchain your project's
`rust-toolchain.toml` (or `rust-toolchain`) names, with the components,
targets and profile it lists. Without one of those either, it installs
`stable`.

```toml
# rust-toolchain.toml
[toolchain]
channel = "1.99"
components = ["rustfmt", "clippy"]
```

Set `toolchain` to install a specific one even when the project has a
toolchain file, such as `stable`, `beta`, `nightly`, `1.99`, `1.99.1` or
`nightly-2026-11-01`. A toolchain named this way, and `stable` by default, is
installed with rustup's minimal profile: `rustc`, `cargo` and the standard
library. List anything else in `components`:

```yaml
- setup-rust:
    with:
      toolchain: nightly
      components: rustfmt,clippy,miri
      target: wasm32-unknown-unknown
```

On a nightly, setup-rust lets rustup fall back to the newest nightly that has
every component you list.

`override` (on by default) runs `rustup override set` in `rust-src-dir`, so
the toolchain is the one cargo uses there whatever the image's rustup has as
its default.

## Flags

| Flag | Default | Description |
|---|---|---|
| `toolchain` | | Toolchain to install. Empty installs the one `rust-toolchain.toml` or `rust-toolchain` in `rust-src-dir` names, else `stable`. |
| `target` | | Targets to add, comma separated, e.g. `wasm32-unknown-unknown`. |
| `components` | | Components to add, comma separated, e.g. `rustfmt,clippy`. |
| `override` | `true` | Make the toolchain the one used in `rust-src-dir`, with `rustup override set`. Ignored when it comes from a toolchain file. |
| `rust-src-dir` | | Directory of the Rust project, where a toolchain file is read and the override is set. Empty is the working directory. |

## How it gets onto PATH

When the image already has rustup on `PATH`, as `cimg/rust` does, setup-rust
uses it. Otherwise it downloads `rustup-init` from `static.rust-lang.org`,
checks it against its published SHA-256, and installs rustup into
`$CARGO_HOME` (`~/.cargo` by default) with no default toolchain, then makes
the toolchain it installs the default.

A step inherits its environment from the job, not from the step before it, so
setup-rust cannot just prepend `~/.cargo/bin` to `PATH`. It links rustup's
proxies (`cargo`, `rustc`, `rustup`, `rustfmt`, `cargo-clippy` and the rest)
into a directory `PATH` already holds, such as `~/.local/bin` or
`/usr/local/bin`, which reaches every later process whatever shell it runs in.
Nothing is written to `$BASH_ENV`.

Binaries that `cargo install` puts in `~/.cargo/bin` run as cargo subcommands
(`cargo nextest`) without being on `PATH`. To run one by its own name, add
`~/.cargo/bin` to `PATH` in your job.

## Platforms

Linux (glibc and musl), macOS and Windows, on x86-64 and arm64.

- On Alpine and other musl distributions, Rust's toolchains need `libgcc`,
  and linking needs a C toolchain: `apk add libgcc gcc musl-dev`.
- On Windows, rustup installs the MSVC toolchain, which needs the Visual
  Studio C++ build tools to link.
