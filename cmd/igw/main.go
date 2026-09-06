// igw operates an Ignition Gateway through one typed CLI core.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alex-mccollum/igw-cli/internal/cli"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := (cli.App{}).Run(ctx, os.Args[1:])
	cancel()
	os.Exit(igwerr.ExitCode(err))
}
