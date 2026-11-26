// Command setup-rust installs a Rust toolchain in a CircleCI job with rustup,
// and puts cargo, rustc and the other rustup proxies on PATH for later steps.
//
// Supported platforms: linux (glibc and musl), darwin, windows; amd64 and arm64.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"
)

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-rust"),
		kong.Description("Install a Rust toolchain with rustup and put cargo and rustc on PATH."),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run()
}
