package cli

import (
	"flag"
	"net/url"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runLogsList(args []string) error {
	fs := flag.NewFlagSet("logs list", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var query stringList
	bindWrapperCommon(fs, &common)
	fs.Var(&query, "query", "Query parameter key=value (repeatable)")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/logs"}
	callArgs = append(callArgs, common.callArgs()...)
	callArgs = appendQueryArgs(callArgs, query)
	return c.runCall(callArgs)
}

func (c *CLI) runLogsDownload(args []string) error {
	fs := flag.NewFlagSet("logs download", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var outPath string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&outPath, "out", "", "Write downloaded logs to file")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/logs/download"}
	callArgs = append(callArgs, common.callArgs()...)
	resolvedOut := chooseDefaultOutPath(outPath, "gateway-logs.zip")
	if resolvedOut != "" {
		callArgs = append(callArgs, "--out", resolvedOut)
	}
	return c.runCall(callArgs)
}

func (c *CLI) runLogsLoggers(args []string) error {
	fs := flag.NewFlagSet("logs loggers", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var query stringList
	bindWrapperCommon(fs, &common)
	fs.Var(&query, "query", "Query parameter key=value (repeatable)")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/logs/loggers"}
	callArgs = append(callArgs, common.callArgs()...)
	callArgs = appendQueryArgs(callArgs, query)
	return c.runCall(callArgs)
}

func (c *CLI) runLogsLoggerSet(args []string) error {
	fs := flag.NewFlagSet("logs logger set", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var name string
	var level string
	var yes bool

	bindWrapperCommon(fs, &common)
	fs.StringVar(&name, "name", "", "Logger name")
	fs.StringVar(&level, "level", "", "Logger level: TRACE|DEBUG|INFO|WARN|ERROR|FATAL|OFF")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return &igwerr.UsageError{Msg: "required: --name"}
	}
	normalizedLevel, err := parseRequiredEnumFlag("level", level, []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "OFF"})
	if err != nil {
		return err
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	path := "/data/api/v1/logs/loggers/" + url.PathEscape(strings.TrimSpace(name))
	callArgs := []string{"--method", "POST", "--path", path, "--yes", "--query", "level=" + normalizedLevel}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}

func (c *CLI) runLogsLevelReset(args []string) error {
	fs := flag.NewFlagSet("logs level-reset", flag.ContinueOnError)
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

	callArgs := []string{"--method", "POST", "--path", "/data/api/v1/logs/levelreset", "--yes"}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}
