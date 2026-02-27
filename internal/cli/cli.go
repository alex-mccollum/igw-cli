package cli

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/buildinfo"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
	"github.com/alex-mccollum/igw-cli/internal/wsl"
)

type CLI struct {
	In              io.Reader
	Out             io.Writer
	Err             io.Writer
	Getenv          func(string) string
	ReadConfig      func() (config.File, error)
	WriteConfig     func(config.File) error
	DetectWSLHostIP func() (string, string, error)
	HTTPClient      *http.Client
	runtime         *runtimeState
}

func New() *CLI {
	return &CLI{
		In:              os.Stdin,
		Out:             os.Stdout,
		Err:             os.Stderr,
		Getenv:          os.Getenv,
		ReadConfig:      config.Read,
		WriteConfig:     config.Write,
		DetectWSLHostIP: wsl.DetectWindowsHostIP,
		runtime:         newRuntimeState(),
	}
}

type rootCommand struct {
	Name        string
	Summary     string
	Subcommands []string
	Run         func(*CLI, []string) error
}

var rootCommandSummaries = map[string]string{
	"api":         "Query local OpenAPI documentation",
	"backup":      "Gateway backup export/restore",
	"call":        "Execute generic Ignition Gateway API request",
	"completion":  "Output shell completion script",
	"config":      "Manage local configuration",
	"diagnostics": "Diagnostics bundle helpers",
	"doctor":      "Check connectivity and auth",
	"exit-codes":  "Print stable machine exit code contract",
	"gateway":     "Convenience gateway commands",
	"logs":        "Gateway log helpers",
	"restart":     "Restart task/gateway helpers",
	"rpc":         "Persistent NDJSON RPC mode for machine callers",
	"scan":        "Convenience scan commands",
	"schema":      "Print machine-readable CLI command schema",
	"tags":        "Tag import/export helpers",
	"wait":        "Wait for operational readiness conditions",
	"version":     "Print build version information",
}

var rootCommands = []rootCommand{
	{Name: "api", Summary: rootCommandSummaries["api"], Subcommands: []string{"list", "show", "search", "tags", "stats", "capability", "sync", "refresh"}, Run: (*CLI).runAPI},
	{Name: "backup", Summary: rootCommandSummaries["backup"], Subcommands: []string{"export", "restore"}, Run: (*CLI).runBackup},
	{Name: "call", Summary: rootCommandSummaries["call"], Run: (*CLI).runCall},
	{Name: "completion", Summary: rootCommandSummaries["completion"], Run: (*CLI).runCompletion},
	{Name: "config", Summary: rootCommandSummaries["config"], Subcommands: []string{"set", "show", "profile"}, Run: (*CLI).runConfig},
	{Name: "diagnostics", Summary: rootCommandSummaries["diagnostics"], Subcommands: []string{"bundle"}, Run: (*CLI).runDiagnostics},
	{Name: "doctor", Summary: rootCommandSummaries["doctor"], Run: (*CLI).runDoctor},
	{Name: "exit-codes", Summary: rootCommandSummaries["exit-codes"], Run: (*CLI).runExitCodes},
	{Name: "gateway", Summary: rootCommandSummaries["gateway"], Subcommands: []string{"info"}, Run: (*CLI).runGateway},
	{Name: "logs", Summary: rootCommandSummaries["logs"], Subcommands: []string{"list", "download", "loggers", "logger", "level-reset"}, Run: (*CLI).runLogs},
	{Name: "restart", Summary: rootCommandSummaries["restart"], Subcommands: []string{"tasks", "gateway"}, Run: (*CLI).runRestart},
	{Name: "rpc", Summary: rootCommandSummaries["rpc"], Run: (*CLI).runRPC},
	{Name: "scan", Summary: rootCommandSummaries["scan"], Subcommands: scanSubcommands, Run: (*CLI).runScan},
	{Name: "schema", Summary: rootCommandSummaries["schema"], Run: (*CLI).runSchema},
	{Name: "tags", Summary: rootCommandSummaries["tags"], Subcommands: []string{"export", "import"}, Run: (*CLI).runTags},
	{Name: "wait", Summary: rootCommandSummaries["wait"], Subcommands: []string{"gateway", "diagnostics-bundle", "restart-tasks"}, Run: (*CLI).runWait},
	{Name: "version", Summary: rootCommandSummaries["version"], Run: (*CLI).runVersion},
}

var completionRootCommands = []string{
	"api", "backup", "call", "completion", "config", "diagnostics", "doctor", "exit-codes", "gateway", "help", "logs", "restart", "rpc", "scan", "schema", "tags", "wait", "version",
}

var completionSubcommands = map[string][]string{
	"api":         {"list", "show", "search", "tags", "stats", "capability", "sync", "refresh"},
	"backup":      {"export", "restore"},
	"config":      {"set", "show", "profile"},
	"diagnostics": {"bundle"},
	"gateway":     {"info"},
	"logs":        {"list", "download", "loggers", "logger", "level-reset"},
	"restart":     {"tasks", "gateway"},
	"scan":        scanSubcommands,
	"tags":        {"export", "import"},
	"wait":        {"gateway", "diagnostics-bundle", "restart-tasks"},
}

var nestedCompletionCommands = map[string][]string{
	"config profile":     {"add", "use", "list"},
	"diagnostics bundle": {"generate", "status", "download"},
	"logs logger":        {"set"},
}

var completionFlags = []string{
	"--profile", "--gateway-url", "--api-key", "--api-key-stdin", "--timeout", "--json", "--timing", "--json-stats", "--include-headers",
	"--spec-file", "--op", "--method", "--path", "--query", "--header", "--body", "--content-type", "--yes",
	"--dry-run", "--retry", "--retry-backoff", "--out", "--batch", "--batch-output", "--parallel", "--select", "--raw", "--compact", "--in", "--provider", "--type", "--collision-policy", "--prefix-depth",
	"--interval", "--wait-timeout", "--openapi-path",
	"--check-write",
	"--workers", "--queue-size",
	"--command",
	"--name", "--level", "--restore-disabled", "--disable-temp-project-backup", "--rename-enabled", "--include-peer-local",
	"--recursive", "--include-udts",
}

func (c *CLI) Execute(args []string) error {
	if len(args) == 0 {
		c.printRootUsage()
		return &igwerr.UsageError{Msg: "required command"}
	}

	command := strings.TrimSpace(args[0])
	switch command {
	case "help", "-h", "--help":
		c.printRootUsage()
		return nil
	case "-v", "--version":
		return c.runVersion(args[1:])
	}

	cmd, ok := findRootCommand(command)
	if !ok {
		c.printRootUsage()
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown command %q", command)}
	}

	return cmd.Run(c, args[1:])
}

func findRootCommand(name string) (rootCommand, bool) {
	for _, cmd := range rootCommands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return rootCommand{}, false
}

func (c *CLI) printRootUsage() {
	fmt.Fprintln(c.Err, "Usage: igw <command> [flags]")
	fmt.Fprintln(c.Err, "")
	fmt.Fprintln(c.Err, "Commands:")
	for _, cmd := range rootCommands {
		fmt.Fprintf(c.Err, "  %-10s %s\n", cmd.Name, cmd.Summary)
	}
}

func (c *CLI) runCompletion(args []string) error {
	if len(args) != 1 {
		return &igwerr.UsageError{Msg: "usage: igw completion <bash>"}
	}

	switch strings.TrimSpace(args[0]) {
	case "bash":
		_, err := io.WriteString(c.Out, bashCompletionScript())
		if err != nil {
			return igwerr.NewTransportError(err)
		}
		return nil
	default:
		return &igwerr.UsageError{Msg: "unsupported shell (supported: bash)"}
	}
}

func (c *CLI) runVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "usage: igw version"}
	}
	fmt.Fprintf(c.Out, "igw version %s\n", buildinfo.Long())
	return nil
}

func bashCompletionScript() string {
	secondLevel := strings.Builder{}
	secondKeys := make([]string, 0, len(completionSubcommands))
	for key := range completionSubcommands {
		secondKeys = append(secondKeys, key)
	}
	sort.Strings(secondKeys)
	for _, key := range secondKeys {
		fmt.Fprintf(&secondLevel, "    %s)\n", key)
		fmt.Fprintf(&secondLevel, "      COMPREPLY=( $(compgen -W \"%s\" -- \"${cur}\") )\n", strings.Join(completionSubcommands[key], " "))
		fmt.Fprintf(&secondLevel, "      return 0\n")
		fmt.Fprintf(&secondLevel, "      ;;\n")
	}

	nestedKeys := make([]string, 0, len(nestedCompletionCommands))
	for key := range nestedCompletionCommands {
		nestedKeys = append(nestedKeys, key)
	}
	sort.Strings(nestedKeys)

	nested := strings.Builder{}
	for _, key := range nestedKeys {
		fmt.Fprintf(&nested, "    \"%s\")\n", key)
		fmt.Fprintf(&nested, "      COMPREPLY=( $(compgen -W \"%s\" -- \"${cur}\") )\n", strings.Join(nestedCompletionCommands[key], " "))
		fmt.Fprintf(&nested, "      return 0\n")
		fmt.Fprintf(&nested, "      ;;\n")
	}

	flags := strings.Join(completionFlags, " ")

	return fmt.Sprintf(`# bash completion for igw
_igw_profiles() {
  igw config profile list 2>/dev/null | awk 'NR>1 {print $2}'
}

_igw_completion() {
  local cur prev cmd1 cmd2
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  cmd1="${COMP_WORDS[1]}"
  cmd2="${COMP_WORDS[2]}"

  case "${prev}" in
    --profile)
      COMPREPLY=( $(compgen -W "$(_igw_profiles)" -- "${cur}") )
      return 0
      ;;
    --method)
      COMPREPLY=( $(compgen -W "GET POST PUT PATCH DELETE HEAD OPTIONS" -- "${cur}") )
      return 0
      ;;
    completion)
      COMPREPLY=( $(compgen -W "bash" -- "${cur}") )
      return 0
      ;;
%s  esac

  case "${cmd1} ${cmd2}" in
%s  esac

  if [[ ${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "%s" -- "${cur}") )
    return 0
  fi

  COMPREPLY=( $(compgen -W "%s" -- "${cur}") )
}

complete -F _igw_completion igw
`, secondLevel.String(), nested.String(), strings.Join(completionRootCommands, " "), flags)
}
