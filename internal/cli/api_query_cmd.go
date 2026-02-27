package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runAPIList(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := newAPIFlagSet("api list", c.Err, jsonRequested)

	var specFile string
	var method string
	var pathContains string
	var jsonOutput bool
	var timing bool
	var jsonStats bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&pathContains, "path-contains", "", "Filter by path substring")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&timing, "timing", false, "Include command timing output")
	fs.BoolVar(&jsonStats, "json-stats", false, "Include runtime stats in JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	start := time.Now()
	ops, err := c.loadAPIOperations(specFile, apiSyncRuntime{Timeout: 8 * time.Second})
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByMethod(ops, method)
	ops = apidocs.FilterByPathContains(ops, pathContains)
	stats := map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"count":     len(ops),
	}

	if jsonOutput {
		payload := map[string]any{"count": len(ops), "operations": ops}
		if timing || jsonStats {
			payload["stats"] = stats
		}
		return writeJSON(c.Out, payload)
	}

	writeOperationTable(c.Out, ops)
	if timing {
		fmt.Fprintf(c.Err, "timing\telapsedMs=%d\n", stats["elapsedMs"])
	}
	return nil
}

func (c *CLI) runAPIShow(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := newAPIFlagSet("api show", c.Err, jsonRequested)

	var specFile string
	var method string
	var path string
	var jsonOutput bool
	var timing bool
	var jsonStats bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&path, "path", "", "Exact API path to show")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&timing, "timing", false, "Include command timing output")
	fs.BoolVar(&jsonStats, "json-stats", false, "Include runtime stats in JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	path, err := applySinglePositionalFallback(fs, path)
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	if strings.TrimSpace(path) == "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "required: --path (or one positional path argument)"})
	}

	start := time.Now()
	ops, err := c.loadAPIOperations(specFile, apiSyncRuntime{Timeout: 8 * time.Second})
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByPath(ops, strings.TrimSpace(path))
	ops = apidocs.FilterByMethod(ops, method)
	stats := map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"count":     len(ops),
	}

	if len(ops) == 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("no API operation found for path %q", path)})
	}

	if jsonOutput {
		payload := map[string]any{"count": len(ops), "operations": ops}
		if timing || jsonStats {
			payload["stats"] = stats
		}
		return writeJSON(c.Out, payload)
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
	if timing {
		fmt.Fprintf(c.Err, "timing\telapsedMs=%d\n", stats["elapsedMs"])
	}

	return nil
}

func (c *CLI) runAPISearch(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := newAPIFlagSet("api search", c.Err, jsonRequested)

	var specFile string
	var method string
	var query string
	var jsonOutput bool
	var timing bool
	var jsonStats bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&query, "query", "", "Search text")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&timing, "timing", false, "Include command timing output")
	fs.BoolVar(&jsonStats, "json-stats", false, "Include runtime stats in JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	query, err := applySinglePositionalFallback(fs, query)
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	if strings.TrimSpace(query) == "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "required: --query (or one positional query argument)"})
	}

	start := time.Now()
	ops, err := c.loadAPIOperations(specFile, apiSyncRuntime{Timeout: 8 * time.Second})
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByMethod(ops, method)
	ops = apidocs.Search(ops, query)
	stats := map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"count":     len(ops),
	}

	if jsonOutput {
		payload := map[string]any{"query": query, "count": len(ops), "operations": ops}
		if timing || jsonStats {
			payload["stats"] = stats
		}
		return writeJSON(c.Out, payload)
	}

	writeOperationTable(c.Out, ops)
	if timing {
		fmt.Fprintf(c.Err, "timing\telapsedMs=%d\n", stats["elapsedMs"])
	}
	return nil
}

func (c *CLI) runAPITags(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := newAPIFlagSet("api tags", c.Err, jsonRequested)

	var specFile string
	var method string
	var pathContains string
	var jsonOutput bool
	var timing bool
	var jsonStats bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&pathContains, "path-contains", "", "Filter by path substring")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&timing, "timing", false, "Include command timing output")
	fs.BoolVar(&jsonStats, "json-stats", false, "Include runtime stats in JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	start := time.Now()
	ops, err := c.loadAPIOperations(specFile, apiSyncRuntime{Timeout: 8 * time.Second})
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByMethod(ops, method)
	ops = apidocs.FilterByPathContains(ops, pathContains)
	tags := apidocs.UniqueTags(ops)
	stats := map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"count":     len(tags),
	}

	if jsonOutput {
		payload := map[string]any{"count": len(tags), "tags": tags}
		if timing || jsonStats {
			payload["stats"] = stats
		}
		return writeJSON(c.Out, payload)
	}

	for _, tag := range tags {
		fmt.Fprintln(c.Out, tag)
	}
	if timing {
		fmt.Fprintf(c.Err, "timing\telapsedMs=%d\n", stats["elapsedMs"])
	}
	return nil
}

func (c *CLI) runAPIStats(args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := newAPIFlagSet("api stats", c.Err, jsonRequested)

	var specFile string
	var method string
	var pathContains string
	var query string
	var prefixDepth int
	var jsonOutput bool
	var timing bool
	var jsonStats bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&pathContains, "path-contains", "", "Filter by path substring")
	fs.StringVar(&query, "query", "", "Search text")
	fs.IntVar(&prefixDepth, "prefix-depth", 0, "Path prefix segment depth for aggregation (0 = auto)")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&timing, "timing", false, "Include command timing output")
	fs.BoolVar(&jsonStats, "json-stats", false, "Include runtime stats in JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}
	if prefixDepth < 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "--prefix-depth must be >= 0"})
	}

	start := time.Now()
	ops, err := c.loadAPIOperations(specFile, apiSyncRuntime{Timeout: 8 * time.Second})
	if err != nil {
		return c.printJSONCommandError(jsonOutput, err)
	}

	ops = apidocs.FilterByMethod(ops, method)
	ops = apidocs.FilterByPathContains(ops, pathContains)
	ops = apidocs.Search(ops, query)
	stats := apidocs.BuildStatsWithPrefixDepth(ops, prefixDepth)
	meta := map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"count":     len(ops),
	}

	if jsonOutput {
		payload := map[string]any{
			"total":        stats.Total,
			"methods":      stats.Methods,
			"tags":         stats.Tags,
			"pathPrefixes": stats.PathPrefixes,
		}
		if timing || jsonStats {
			payload["stats"] = meta
		}
		return writeJSON(c.Out, payload)
	}

	writeStatsTable(c.Out, stats)
	if timing {
		fmt.Fprintf(c.Err, "timing\telapsedMs=%d\n", meta["elapsedMs"])
	}
	return nil
}

func newAPIFlagSet(name string, errOut io.Writer, jsonRequested bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}
	return fs
}

func applySinglePositionalFallback(fs *flag.FlagSet, current string) (string, error) {
	if fs.NArg() == 0 {
		return current, nil
	}
	if strings.TrimSpace(current) == "" && fs.NArg() == 1 {
		return fs.Arg(0), nil
	}
	return "", &igwerr.UsageError{Msg: "unexpected positional arguments"}
}

func writeOperationTable(out io.Writer, ops []apidocs.Operation) {
	fmt.Fprintln(out, "METHOD\tPATH\tOPERATION_ID\tSUMMARY")
	for _, op := range ops {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", op.Method, op.Path, op.OperationID, op.Summary)
	}
}

func writeStatsTable(out io.Writer, stats apidocs.Stats) {
	fmt.Fprintf(out, "total\t%d\n", stats.Total)
	fmt.Fprintln(out, "METHOD\tCOUNT")
	for _, row := range stats.Methods {
		fmt.Fprintf(out, "%s\t%d\n", row.Name, row.Count)
	}

	fmt.Fprintln(out, "TAG\tCOUNT")
	for _, row := range stats.Tags {
		fmt.Fprintf(out, "%s\t%d\n", row.Name, row.Count)
	}

	fmt.Fprintln(out, "PATH_PREFIX\tCOUNT")
	for _, row := range stats.PathPrefixes {
		fmt.Fprintf(out, "%s\t%d\n", row.Name, row.Count)
	}
}
