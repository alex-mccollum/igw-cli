package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runAPI(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw api <list|show|search> [flags]")
		return &igwerr.UsageError{Msg: "required api subcommand"}
	}

	switch args[0] {
	case "list":
		return c.runAPIList(args[1:])
	case "show":
		return c.runAPIShow(args[1:])
	case "search":
		return c.runAPISearch(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown api subcommand %q", args[0])}
	}
}

func (c *CLI) runAPIList(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := flag.NewFlagSet("api list", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var specFile string
	var method string
	var pathContains string
	var jsonOutput bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&pathContains, "path-contains", "", "Filter by path substring")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	ops, err := loadAPIOperations(specFile)
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByMethod(ops, method)
	ops = apidocs.FilterByPathContains(ops, pathContains)

	if jsonOutput {
		return writeJSON(c.Out, map[string]any{
			"count":      len(ops),
			"operations": ops,
		})
	}

	fmt.Fprintln(c.Out, "METHOD\tPATH\tOPERATION_ID\tSUMMARY")
	for _, op := range ops {
		fmt.Fprintf(c.Out, "%s\t%s\t%s\t%s\n", op.Method, op.Path, op.OperationID, op.Summary)
	}

	return nil
}

func (c *CLI) runAPIShow(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := flag.NewFlagSet("api show", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var specFile string
	var method string
	var path string
	var jsonOutput bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&path, "path", "", "Exact API path to show")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		if strings.TrimSpace(path) == "" && fs.NArg() == 1 {
			path = fs.Arg(0)
		} else {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
		}
	}

	if strings.TrimSpace(path) == "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "required: --path (or one positional path argument)"})
	}

	ops, err := loadAPIOperations(specFile)
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByPath(ops, strings.TrimSpace(path))
	ops = apidocs.FilterByMethod(ops, method)

	if len(ops) == 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("no API operation found for path %q", path)})
	}

	if jsonOutput {
		return writeJSON(c.Out, map[string]any{
			"count":      len(ops),
			"operations": ops,
		})
	}

	for i, op := range ops {
		if i > 0 {
			fmt.Fprintln(c.Out)
		}
		fmt.Fprintf(c.Out, "method\t%s\n", op.Method)
		fmt.Fprintf(c.Out, "path\t%s\n", op.Path)
		fmt.Fprintf(c.Out, "operation_id\t%s\n", op.OperationID)
		fmt.Fprintf(c.Out, "summary\t%s\n", op.Summary)
		fmt.Fprintf(c.Out, "deprecated\t%t\n", op.Deprecated)
		if len(op.Tags) > 0 {
			fmt.Fprintf(c.Out, "tags\t%s\n", strings.Join(op.Tags, ","))
		}
		if strings.TrimSpace(op.Description) != "" {
			fmt.Fprintf(c.Out, "description\t%s\n", op.Description)
		}
	}

	return nil
}

func (c *CLI) runAPISearch(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := flag.NewFlagSet("api search", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var specFile string
	var method string
	var query string
	var jsonOutput bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&query, "query", "", "Search text")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		if strings.TrimSpace(query) == "" && fs.NArg() == 1 {
			query = fs.Arg(0)
		} else {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
		}
	}

	if strings.TrimSpace(query) == "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "required: --query (or one positional query argument)"})
	}

	ops, err := loadAPIOperations(specFile)
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByMethod(ops, method)
	ops = apidocs.Search(ops, query)

	if jsonOutput {
		return writeJSON(c.Out, map[string]any{
			"query":      query,
			"count":      len(ops),
			"operations": ops,
		})
	}

	fmt.Fprintln(c.Out, "METHOD\tPATH\tOPERATION_ID\tSUMMARY")
	for _, op := range ops {
		fmt.Fprintf(c.Out, "%s\t%s\t%s\t%s\n", op.Method, op.Path, op.OperationID, op.Summary)
	}

	return nil
}

func loadAPIOperations(specFile string) ([]apidocs.Operation, error) {
	resolvedSpecFile, candidates := resolveSpecFile(specFile)
	ops, err := apidocs.LoadOperations(resolvedSpecFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if len(candidates) > 1 {
				return nil, &igwerr.UsageError{
					Msg: fmt.Sprintf("OpenAPI spec not found. checked: %q (cwd), %q (config). pass --spec-file /path/to/openapi.json", candidates[0], candidates[1]),
				}
			}
			return nil, &igwerr.UsageError{
				Msg: fmt.Sprintf("OpenAPI spec not found at %q (pass --spec-file /path/to/openapi.json)", resolvedSpecFile),
			}
		}
		return nil, &igwerr.UsageError{Msg: err.Error()}
	}

	return ops, nil
}

func formatOperationMatches(ops []apidocs.Operation) string {
	if len(ops) == 0 {
		return ""
	}

	limit := len(ops)
	if limit > 3 {
		limit = 3
	}

	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%s %s", ops[i].Method, ops[i].Path))
	}
	if len(ops) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(ops)-limit))
	}

	return strings.Join(parts, "; ")
}

func resolveSpecFile(specFile string) (string, []string) {
	specFile = strings.TrimSpace(specFile)
	if specFile != "" && specFile != apidocs.DefaultSpecFile {
		return specFile, []string{specFile}
	}

	candidates := []string{apidocs.DefaultSpecFile}
	if cfgDir, err := config.Dir(); err == nil {
		candidates = append(candidates, filepath.Join(cfgDir, apidocs.DefaultSpecFile))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, candidates
		}
	}

	return candidates[0], candidates
}
