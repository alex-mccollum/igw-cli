package cli

import "flag"

const (
	scanSubcommandProjects = "projects"
	scanSubcommandConfig   = "config"
)

var scanSubcommands = []string{scanSubcommandProjects, scanSubcommandConfig}

const (
	scanProjectsPath = "/data/api/v1/scan/projects"
	scanConfigPath   = "/data/api/v1/scan/config"
)

func (c *CLI) runScanConfig(args []string) error {
	return c.runScanMutation("scan config", scanConfigPath, args)
}

func (c *CLI) runScanProjects(args []string) error {
	return c.runScanMutation("scan projects", scanProjectsPath, args)
}

func (c *CLI) runScanMutation(commandName string, path string, args []string) error {
	fs := flag.NewFlagSet(commandName, flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var yes bool
	var dryRun bool
	bindWrapperCommon(fs, &common)
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")
	fs.BoolVar(&dryRun, "dry-run", false, "Append dryRun=true query parameter")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{
		"--method", "POST",
		"--path", path,
		"--timeout", common.timeout.String(),
	}
	callArgs = append(callArgs, common.callArgsExcludingTimeout()...)
	if dryRun {
		callArgs = append(callArgs, "--dry-run")
	}
	if yes {
		callArgs = append(callArgs, "--yes")
	}

	return c.runCall(callArgs)
}
