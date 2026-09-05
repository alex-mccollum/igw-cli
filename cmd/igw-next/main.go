// igw-next is the development entrypoint for the staged v1 rebuild.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
	"github.com/alex-mccollum/igw-cli/internal/nextcli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := (nextcli.App{}).Run(ctx, os.Args[1:])
	cancel()
	os.Exit(igwerr.ExitCode(err))
}
