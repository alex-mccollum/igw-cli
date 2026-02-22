package cli

import "flag"

func (c *CLI) runScanProjects(args []string) error {
	fs := flag.NewFlagSet("scan projects", flag.ContinueOnError)
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
		"--path", "/data/api/v1/scan/projects",
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
