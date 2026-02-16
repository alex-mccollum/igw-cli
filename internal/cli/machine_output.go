package cli

import (
	"strconv"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func argsWantJSON(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--json":
			return true
		case strings.HasPrefix(arg, "--json="):
			raw := strings.TrimSpace(strings.TrimPrefix(arg, "--json="))
			if raw == "" {
				continue
			}
			if parsed, err := strconv.ParseBool(raw); err == nil {
				return parsed
			}
		}
	}
	return false
}

func (c *CLI) printJSONCommandError(jsonOutput bool, err error) error {
	if jsonOutput {
		_ = writeJSON(c.Out, map[string]any{
			"ok":    false,
			"code":  igwerr.ExitCode(err),
			"error": err.Error(),
		})
	}
	return err
}
