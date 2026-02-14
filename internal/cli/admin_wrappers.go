package cli

import (
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runLogs(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw logs <list|download|loggers|logger|level-reset> [flags]")
		return &igwerr.UsageError{Msg: "required logs subcommand"}
	}

	switch args[0] {
	case "list":
		return c.runLogsList(args[1:])
	case "download":
		return c.runLogsDownload(args[1:])
	case "loggers":
		return c.runLogsLoggers(args[1:])
	case "logger":
		return c.runLogsLogger(args[1:])
	case "level-reset":
		return c.runLogsLevelReset(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown logs subcommand %q", args[0])}
	}
}

func (c *CLI) runDiagnostics(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw diagnostics bundle <generate|status|download> [flags]")
		return &igwerr.UsageError{Msg: "required diagnostics subcommand"}
	}
	if args[0] != "bundle" {
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown diagnostics subcommand %q", args[0])}
	}
	if len(args) < 2 {
		fmt.Fprintln(c.Err, "Usage: igw diagnostics bundle <generate|status|download> [flags]")
		return &igwerr.UsageError{Msg: "required diagnostics bundle subcommand"}
	}

	switch args[1] {
	case "generate":
		return c.runDiagnosticsBundleGenerate(args[2:])
	case "status":
		return c.runDiagnosticsBundleStatus(args[2:])
	case "download":
		return c.runDiagnosticsBundleDownload(args[2:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown diagnostics bundle subcommand %q", args[1])}
	}
}

func (c *CLI) runBackup(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw backup <export|restore> [flags]")
		return &igwerr.UsageError{Msg: "required backup subcommand"}
	}

	switch args[0] {
	case "export":
		return c.runBackupExport(args[1:])
	case "restore":
		return c.runBackupRestore(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown backup subcommand %q", args[0])}
	}
}

func (c *CLI) runTags(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw tags <export|import> [flags]")
		return &igwerr.UsageError{Msg: "required tags subcommand"}
	}

	switch args[0] {
	case "export":
		return c.runTagsExport(args[1:])
	case "import":
		return c.runTagsImport(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown tags subcommand %q", args[0])}
	}
}

func (c *CLI) runRestart(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw restart <tasks|gateway> [flags]")
		return &igwerr.UsageError{Msg: "required restart subcommand"}
	}

	switch args[0] {
	case "tasks":
		return c.runRestartTasks(args[1:])
	case "gateway":
		return c.runRestartGateway(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown restart subcommand %q", args[0])}
	}
}

func (c *CLI) runLogsList(args []string) error {
	fs := flag.NewFlagSet("logs list", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var query stringList
	bindWrapperCommon(fs, &common)
	fs.Var(&query, "query", "Query parameter key=value (repeatable)")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/logs/download"}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(outPath) != "" {
		callArgs = append(callArgs, "--out", outPath)
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/logs/loggers"}
	callArgs = append(callArgs, common.callArgs()...)
	callArgs = appendQueryArgs(callArgs, query)
	return c.runCall(callArgs)
}

func (c *CLI) runLogsLogger(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw logs logger set --name <logger> --level <LEVEL> --yes [flags]")
		return &igwerr.UsageError{Msg: "required logs logger subcommand"}
	}

	switch args[0] {
	case "set":
		return c.runLogsLoggerSet(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown logs logger subcommand %q", args[0])}
	}
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	if strings.TrimSpace(name) == "" {
		return &igwerr.UsageError{Msg: "required: --name"}
	}
	if strings.TrimSpace(level) == "" {
		return &igwerr.UsageError{Msg: "required: --level"}
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	path := "/data/api/v1/logs/loggers/" + url.PathEscape(strings.TrimSpace(name))
	callArgs := []string{"--method", "POST", "--path", path, "--yes", "--query", "level=" + strings.TrimSpace(level)}
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	callArgs := []string{"--method", "POST", "--path", "/data/api/v1/logs/levelreset", "--yes"}
	callArgs = append(callArgs, common.callArgs()...)
	return c.runCall(callArgs)
}

func (c *CLI) runDiagnosticsBundleGenerate(args []string) error {
	fs := flag.NewFlagSet("diagnostics bundle generate", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var yes bool
	bindWrapperCommon(fs, &common)
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/diagnostics/bundle/download"}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(outPath) != "" {
		callArgs = append(callArgs, "--out", outPath)
	}
	return c.runCall(callArgs)
}

func (c *CLI) runBackupExport(args []string) error {
	fs := flag.NewFlagSet("backup export", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var outPath string
	var includePeerLocal string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&outPath, "out", "", "Write gateway backup (.gwbk) to file")
	fs.StringVar(&includePeerLocal, "include-peer-local", "", "Set includePeerLocal query to true/false")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{"--method", "GET", "--path", "/data/api/v1/backup"}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(includePeerLocal) != "" {
		callArgs = append(callArgs, "--query", "includePeerLocal="+strings.TrimSpace(includePeerLocal))
	}
	if strings.TrimSpace(outPath) != "" {
		callArgs = append(callArgs, "--out", outPath)
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	if strings.TrimSpace(inPath) == "" {
		return &igwerr.UsageError{Msg: "required: --in"}
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/backup",
		"--body", "@" + inPath,
		"--content-type", "application/octet-stream",
		"--yes",
	}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(restoreDisabled) != "" {
		callArgs = append(callArgs, "--query", "restoreDisabled="+strings.TrimSpace(restoreDisabled))
	}
	if strings.TrimSpace(disableTempProjectBackup) != "" {
		callArgs = append(callArgs, "--query", "disableTempProjectBackup="+strings.TrimSpace(disableTempProjectBackup))
	}
	if strings.TrimSpace(renameEnabled) != "" {
		callArgs = append(callArgs, "--query", "renameEnabled="+strings.TrimSpace(renameEnabled))
	}
	return c.runCall(callArgs)
}

func (c *CLI) runTagsExport(args []string) error {
	fs := flag.NewFlagSet("tags export", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var provider string
	var exportType string
	var rootPath string
	var recursive string
	var includeUdts string
	var outPath string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&provider, "provider", "", "Tag provider name")
	fs.StringVar(&exportType, "type", "", "Export type: json|xml")
	fs.StringVar(&rootPath, "path", "", "Root tag path")
	fs.StringVar(&recursive, "recursive", "", "Set recursive query to true/false")
	fs.StringVar(&includeUdts, "include-udts", "", "Set includeUdts query to true/false")
	fs.StringVar(&outPath, "out", "", "Write tag export to file")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	if strings.TrimSpace(provider) == "" {
		return &igwerr.UsageError{Msg: "required: --provider"}
	}
	if strings.TrimSpace(exportType) == "" {
		return &igwerr.UsageError{Msg: "required: --type"}
	}

	callArgs := []string{
		"--method", "GET",
		"--path", "/data/api/v1/tags/export",
		"--query", "provider=" + strings.TrimSpace(provider),
		"--query", "type=" + strings.TrimSpace(exportType),
	}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(rootPath) != "" {
		callArgs = append(callArgs, "--query", "path="+strings.TrimSpace(rootPath))
	}
	if strings.TrimSpace(recursive) != "" {
		callArgs = append(callArgs, "--query", "recursive="+strings.TrimSpace(recursive))
	}
	if strings.TrimSpace(includeUdts) != "" {
		callArgs = append(callArgs, "--query", "includeUdts="+strings.TrimSpace(includeUdts))
	}
	if strings.TrimSpace(outPath) != "" {
		callArgs = append(callArgs, "--out", outPath)
	}
	return c.runCall(callArgs)
}

func (c *CLI) runTagsImport(args []string) error {
	fs := flag.NewFlagSet("tags import", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var provider string
	var importType string
	var collisionPolicy string
	var rootPath string
	var inPath string
	var yes bool
	bindWrapperCommon(fs, &common)
	fs.StringVar(&provider, "provider", "", "Tag provider name")
	fs.StringVar(&importType, "type", "", "Import type: json|xml|csv")
	fs.StringVar(&collisionPolicy, "collision-policy", "", "Collision policy: Abort|Overwrite|Rename|Ignore|MergeOverwrite")
	fs.StringVar(&rootPath, "path", "", "Root tag path")
	fs.StringVar(&inPath, "in", "", "Path to tag import file")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	if strings.TrimSpace(provider) == "" {
		return &igwerr.UsageError{Msg: "required: --provider"}
	}
	if strings.TrimSpace(importType) == "" {
		return &igwerr.UsageError{Msg: "required: --type"}
	}
	if strings.TrimSpace(collisionPolicy) == "" {
		return &igwerr.UsageError{Msg: "required: --collision-policy"}
	}
	if strings.TrimSpace(inPath) == "" {
		return &igwerr.UsageError{Msg: "required: --in"}
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/tags/import",
		"--query", "provider=" + strings.TrimSpace(provider),
		"--query", "type=" + strings.TrimSpace(importType),
		"--query", "collisionPolicy=" + strings.TrimSpace(collisionPolicy),
		"--body", "@" + inPath,
		"--content-type", "application/octet-stream",
		"--yes",
	}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(rootPath) != "" {
		callArgs = append(callArgs, "--query", "path="+strings.TrimSpace(rootPath))
	}
	return c.runCall(callArgs)
}

func (c *CLI) runRestartTasks(args []string) error {
	fs := flag.NewFlagSet("restart tasks", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	bindWrapperCommon(fs, &common)

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
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

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
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

type wrapperCommon struct {
	gatewayURL     string
	apiKey         string
	apiKeyStdin    bool
	profile        string
	timeout        time.Duration
	jsonOutput     bool
	includeHeaders bool
}

func bindWrapperCommon(fs *flag.FlagSet, common *wrapperCommon) {
	fs.StringVar(&common.gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&common.apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&common.apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&common.profile, "profile", "", "Config profile name")
	fs.DurationVar(&common.timeout, "timeout", 8*time.Second, "Request timeout")
	fs.BoolVar(&common.jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&common.includeHeaders, "include-headers", false, "Include response headers")
}

func (w wrapperCommon) callArgs() []string {
	args := []string{
		"--timeout", w.timeout.String(),
	}
	if strings.TrimSpace(w.gatewayURL) != "" {
		args = append(args, "--gateway-url", strings.TrimSpace(w.gatewayURL))
	}
	if strings.TrimSpace(w.apiKey) != "" {
		args = append(args, "--api-key", strings.TrimSpace(w.apiKey))
	}
	if w.apiKeyStdin {
		args = append(args, "--api-key-stdin")
	}
	if strings.TrimSpace(w.profile) != "" {
		args = append(args, "--profile", strings.TrimSpace(w.profile))
	}
	if w.jsonOutput {
		args = append(args, "--json")
	}
	if w.includeHeaders {
		args = append(args, "--include-headers")
	}
	return args
}

func appendQueryArgs(args []string, query []string) []string {
	for _, pair := range query {
		args = append(args, "--query", pair)
	}
	return args
}
