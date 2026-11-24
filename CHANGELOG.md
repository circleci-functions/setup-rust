# Changelog

All notable changes to setup-rust will be documented in this file. Each release's section is added
by its release PR (see [RELEASE.md](RELEASE.md)) and becomes its GitHub release notes.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-11-24

First release: installs a Rust toolchain with rustup, from the flags or a rust-toolchain file, with the
components and targets asked for, puts cargo and rustc on PATH, and caches cargo's downloads and the
build in the job cache.
