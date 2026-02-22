package cli

import (
	"flag"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runBackupExport(args []string) error {
	fs := flag.NewFlagSet("backup export", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var outPath string
	var includePeerLocal string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&outPath, "out", "", "Write gateway backup (.gwbk) to file")
	fs.StringVar(&includePeerLocal, "include-peer-local", "", "Set includePeerLocal query to true/false")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	normalizedIncludePeerLocal, err := parseOptionalBoolFlag("include-peer-local", includePeerLocal)
	if err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/backup"}
	callArgs = append(callArgs, common.callArgs()...)
	if normalizedIncludePeerLocal != "" {
		callArgs = append(callArgs, "--query", "includePeerLocal="+normalizedIncludePeerLocal)
	}
	resolvedOut := chooseDefaultOutPath(outPath, "gateway.gwbk")
	if resolvedOut != "" {
		callArgs = append(callArgs, "--out", resolvedOut)
	}
	return c.runCall(callArgs)
}

func (c *CLI) runBackupRestore(args []string) error {
	fs := flag.NewFlagSet("backup restore", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var inPath string
	var yes bool
	var restoreDisabled string
	var disableTempProjectBackup string
	var renameEnabled string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&inPath, "in", "", "Path to .gwbk file")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")
	fs.StringVar(&restoreDisabled, "restore-disabled", "", "Set restoreDisabled query to true/false")
	fs.StringVar(&disableTempProjectBackup, "disable-temp-project-backup", "", "Set disableTempProjectBackup query to true/false")
	fs.StringVar(&renameEnabled, "rename-enabled", "", "Set renameEnabled query to true/false")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(inPath) == "" {
		return &igwerr.UsageError{Msg: "required: --in"}
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	normalizedRestoreDisabled, err := parseOptionalBoolFlag("restore-disabled", restoreDisabled)
	if err != nil {
		return err
	}
	normalizedDisableTempProjectBackup, err := parseOptionalBoolFlag("disable-temp-project-backup", disableTempProjectBackup)
	if err != nil {
		return err
	}
	normalizedRenameEnabled, err := parseOptionalBoolFlag("rename-enabled", renameEnabled)
	if err != nil {
		return err
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/backup",
		"--body", "@" + inPath,
		"--content-type", "application/octet-stream",
		"--yes",
	}
	callArgs = append(callArgs, common.callArgs()...)
	if normalizedRestoreDisabled != "" {
		callArgs = append(callArgs, "--query", "restoreDisabled="+normalizedRestoreDisabled)
	}
	if normalizedDisableTempProjectBackup != "" {
		callArgs = append(callArgs, "--query", "disableTempProjectBackup="+normalizedDisableTempProjectBackup)
	}
	if normalizedRenameEnabled != "" {
		callArgs = append(callArgs, "--query", "renameEnabled="+normalizedRenameEnabled)
	}
	return c.runCall(callArgs)
}
