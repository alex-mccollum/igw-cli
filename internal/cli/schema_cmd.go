package cli

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/buildinfo"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type schemaDoc struct {
	SchemaVersion int            `json:"schemaVersion"`
	Version       string         `json:"version"`
	Command       schemaCommand  `json:"command"`
	GlobalFlags   []string       `json:"globalFlags"`
	ExitCodes     map[string]int `json:"exitCodes"`
}

type schemaCommand struct {
	Name        string          `json:"name"`
	Summary     string          `json:"summary,omitempty"`
	Path        string          `json:"path"`
	Subcommands []schemaCommand `json:"subcommands,omitempty"`
}

func (c *CLI) runSchema(args []string) error {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var compact bool
	var raw bool
	var selectors stringList
	fs.BoolVar(&compact, "compact", false, "Print compact one-line JSON")
	fs.BoolVar(&raw, "raw", false, "Print selected value without JSON quoting (requires one --select)")
	fs.Var(&selectors, "select", "Select JSON path from output (repeatable)")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(true, &igwerr.UsageError{Msg: err.Error()})
	}
	selectOpts, selectErr := newJSONSelectOptions(true, compact, raw, selectors)
	if selectErr != nil {
		return c.printJSONCommandError(true, selectErr)
	}

	root := buildSchemaRoot()
	selected := root
	if fs.NArg() > 0 {
		pathTokens := make([]string, 0, fs.NArg())
		for _, token := range fs.Args() {
			trimmed := strings.TrimSpace(token)
			if trimmed != "" {
				pathTokens = append(pathTokens, trimmed)
			}
		}
		if len(pathTokens) > 0 {
			node, ok := findSchemaCommand(root, pathTokens)
			if !ok {
				return c.printJSONCommandError(true, &igwerr.UsageError{
					Msg: fmt.Sprintf("unknown command path %q", strings.Join(pathTokens, " ")),
				})
			}
			selected = node
		}
	}

	payload := schemaDoc{
		SchemaVersion: 1,
		Version:       buildinfo.Long(),
		Command:       selected,
		GlobalFlags:   slices.Clone(completionFlags),
		ExitCodes:     stableExitCodeMap(),
	}
	if err := printJSONSelection(c.Out, payload, selectOpts); err != nil {
		return c.printJSONCommandError(true, err)
	}
	return nil
}

func buildSchemaRoot() schemaCommand {
	root := schemaCommand{
		Name: "igw",
		Path: "igw",
	}

	for _, name := range completionRootCommands {
		if name == "help" {
			continue
		}
		node := schemaCommand{
			Name:    name,
			Summary: schemaCommandSummaries[name],
			Path:    root.Path + " " + name,
		}
		for _, sub := range completionSubcommands[name] {
			node.Subcommands = append(node.Subcommands, schemaCommand{
				Name: sub,
				Path: node.Path + " " + sub,
			})
		}
		root.Subcommands = append(root.Subcommands, node)
	}

	for chain, subs := range nestedCompletionCommands {
		parent, ok := findSchemaCommandPtr(&root, strings.Fields(chain))
		if !ok {
			continue
		}
		for _, sub := range subs {
			if hasSchemaSubcommand(*parent, sub) {
				continue
			}
			parent.Subcommands = append(parent.Subcommands, schemaCommand{
				Name: sub,
				Path: parent.Path + " " + sub,
			})
		}
	}

	sortSchemaCommands(&root)
	return root
}

var schemaCommandSummaries = map[string]string{
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
	"version":     "Print build version information",
	"wait":        "Wait for operational readiness conditions",
}

func sortSchemaCommands(node *schemaCommand) {
	if node == nil || len(node.Subcommands) == 0 {
		return
	}
	slices.SortFunc(node.Subcommands, func(a schemaCommand, b schemaCommand) int {
		return strings.Compare(a.Name, b.Name)
	})
	for i := range node.Subcommands {
		sortSchemaCommands(&node.Subcommands[i])
	}
}

func hasSchemaSubcommand(node schemaCommand, name string) bool {
	for _, sub := range node.Subcommands {
		if strings.EqualFold(sub.Name, name) {
			return true
		}
	}
	return false
}

func findSchemaCommand(root schemaCommand, path []string) (schemaCommand, bool) {
	cur, ok := findSchemaCommandPtr(&root, path)
	if !ok || cur == nil {
		return schemaCommand{}, false
	}
	return *cur, true
}

func findSchemaCommandPtr(root *schemaCommand, path []string) (*schemaCommand, bool) {
	if root == nil {
		return nil, false
	}
	cur := root
	for _, token := range path {
		found := false
		for i := range cur.Subcommands {
			sub := &cur.Subcommands[i]
			if strings.EqualFold(sub.Name, token) {
				cur = sub
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return cur, true
}
