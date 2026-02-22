package cli

import (
	"flag"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runDiagnosticsBundleGenerate(args []string) error {
	fs := flag.NewFlagSet("diagnostics bundle generate", flag.ContinueOnError)
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

	callArgs := []string{"--method", "POST", "--path", "/data/api/v1/diagnostics/bundle/generate", "--yes"}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}

func (c *CLI) runDiagnosticsBundleStatus(args []string) error {
	fs := flag.NewFlagSet("diagnostics bundle status", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	bindWrapperCommon(fs, &common)

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/diagnostics/bundle/status"}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}

func (c *CLI) runDiagnosticsBundleDownload(args []string) error {
	fs := flag.NewFlagSet("diagnostics bundle download", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var outPath string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&outPath, "out", "", "Write diagnostics bundle to file")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/diagnostics/bundle/download"}
	callArgs = append(callArgs, common.callArgs()...)
	resolvedOut := chooseDefaultOutPath(outPath, "diagnostics.zip")
	if resolvedOut != "" {
		callArgs = append(callArgs, "--out", resolvedOut)
	}
	return c.runCall(callArgs)
}
