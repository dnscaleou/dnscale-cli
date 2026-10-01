package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dnscaleou/dnscale-cli/internal/cli"
)

var version = "1.0.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version)
	stop()
	os.Exit(code)
}
