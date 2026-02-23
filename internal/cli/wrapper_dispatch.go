package cli

import (
	"fmt"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runLogs(args []string) error {
	return c.runWrapperSubcommand(
		args,
		"Usage: igw logs <list|download|loggers|logger|level-reset> [flags]",
		"required logs subcommand",
		"unknown logs subcommand %q",
		map[string]func([]string) error{
			"list":        c.runLogsList,
			"download":    c.runLogsDownload,
			"loggers":     c.runLogsLoggers,
			"logger":      c.runLogsLogger,
			"level-reset": c.runLogsLevelReset,
		},
	)
}

func (c *CLI) runDiagnostics(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw diagnostics bundle <generate|status|download> [flags]")
		return &igwerr.UsageError{Msg: "required diagnostics subcommand"}
	}
	if args[0] != "bundle" {
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown diagnostics subcommand %q", args[0])}
	}

	return c.runWrapperSubcommand(
		args[1:],
		"Usage: igw diagnostics bundle <generate|status|download> [flags]",
		"required diagnostics bundle subcommand",
		"unknown diagnostics bundle subcommand %q",
		map[string]func([]string) error{
			"generate": c.runDiagnosticsBundleGenerate,
			"status":   c.runDiagnosticsBundleStatus,
			"download": c.runDiagnosticsBundleDownload,
		},
	)
}

func (c *CLI) runBackup(args []string) error {
	return c.runWrapperSubcommand(
		args,
		"Usage: igw backup <export|restore> [flags]",
		"required backup subcommand",
		"unknown backup subcommand %q",
		map[string]func([]string) error{
			"export":  c.runBackupExport,
			"restore": c.runBackupRestore,
		},
	)
}

func (c *CLI) runTags(args []string) error {
	return c.runWrapperSubcommand(
		args,
		"Usage: igw tags <export|import> [flags]",
		"required tags subcommand",
		"unknown tags subcommand %q",
		map[string]func([]string) error{
			"export": c.runTagsExport,
			"import": c.runTagsImport,
		},
	)
}

func (c *CLI) runRestart(args []string) error {
	return c.runWrapperSubcommand(
		args,
		"Usage: igw restart <tasks|gateway> [flags]",
		"required restart subcommand",
		"unknown restart subcommand %q",
		map[string]func([]string) error{
			"tasks":   c.runRestartTasks,
			"gateway": c.runRestartGateway,
		},
	)
}

func (c *CLI) runGateway(args []string) error {
	return c.runWrapperSubcommand(
		args,
		"Usage: igw gateway <info> [flags]",
		"required gateway subcommand",
		"unknown gateway subcommand %q",
		map[string]func([]string) error{
			"info": c.runGatewayInfo,
		},
	)
}

func (c *CLI) runScan(args []string) error {
	// Keep `scan` shorthand mapped to the most common project scan operation.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return c.runScanProjects(args)
	}
	return c.runWrapperSubcommand(
		args,
		"Usage: igw scan <projects|config> [flags]",
		"required scan subcommand",
		"unknown scan subcommand %q",
		map[string]func([]string) error{
			scanSubcommandProjects: c.runScanProjects,
			scanSubcommandConfig:   c.runScanConfig,
		},
	)
}

func (c *CLI) runLogsLogger(args []string) error {
	return c.runWrapperSubcommand(
		args,
		"Usage: igw logs logger set --name <logger> --level <LEVEL> --yes [flags]",
		"required logs logger subcommand",
		"unknown logs logger subcommand %q",
		map[string]func([]string) error{
			"set": c.runLogsLoggerSet,
		},
	)
}

func (c *CLI) runWrapperSubcommand(args []string, usage string, requiredMsg string, unknownFmt string, handlers map[string]func([]string) error) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, usage)
		return &igwerr.UsageError{Msg: requiredMsg}
	}

	handler, ok := handlers[args[0]]
	if !ok {
		return &igwerr.UsageError{Msg: fmt.Sprintf(unknownFmt, args[0])}
	}

	return handler(args[1:])
}
