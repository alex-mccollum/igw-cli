package cli

import (
	"flag"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runRestartTasks(args []string) error {
	fs := flag.NewFlagSet("restart tasks", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	bindWrapperCommon(fs, &common)

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/restart-tasks/pending"}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}

func (c *CLI) runRestartGateway(args []string) error {
	fs := flag.NewFlagSet("restart gateway", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var yes bool
	bindWrapperCommon(fs, &common)
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/restart-tasks/restart",
		"--query", "confirm=true",
		"--yes",
	}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}
