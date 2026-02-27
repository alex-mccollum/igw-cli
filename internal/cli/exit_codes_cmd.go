package cli

import (
	"flag"
	"fmt"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runExitCodes(args []string) error {
	jsonRequested := argsWantJSON(args)

	fs := flag.NewFlagSet("exit-codes", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var jsonOutput bool
	var compact bool
	var raw bool
	var selectors stringList
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&compact, "compact", false, "Print compact one-line JSON")
	fs.BoolVar(&raw, "raw", false, "Print selected value without JSON quoting (requires one --select)")
	fs.Var(&selectors, "select", "Select JSON path from output (repeatable)")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}

	selectOpts, selectErr := newJSONSelectOptions(jsonOutput, compact, raw, selectors)
	if selectErr != nil {
		return c.printJSONCommandError(jsonOutput, selectErr)
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "usage: igw exit-codes [--json]"})
	}

	entries := stableExitCodeEntries()
	if !jsonOutput {
		for _, entry := range entries {
			fmt.Fprintf(c.Out, "%s\t%d\n", entry.Name, entry.Code)
		}
		return nil
	}

	payload := map[string]any{
		"ok":        true,
		"exitCodes": stableExitCodeMap(),
	}
	if err := printJSONSelection(c.Out, payload, selectOpts); err != nil {
		return c.printJSONCommandError(true, err)
	}
	return nil
}
