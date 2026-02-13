package main

import (
	"os"

	"github.com/alex-mccollum/igw-cli/internal/cli"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func main() {
	c := cli.New()
	err := c.Execute(os.Args[1:])
	os.Exit(igwerr.ExitCode(err))
}
