package cli

import (
	"errors"
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
		_ = writeJSON(c.Out, jsonErrorPayload(err))
	}
	return err
}

func jsonErrorPayload(err error) map[string]any {
	payload := map[string]any{
		"ok":    false,
		"code":  igwerr.ExitCode(err),
		"error": err.Error(),
	}

	details := map[string]any{}

	var statusErr *igwerr.StatusError
	if errors.As(err, &statusErr) {
		details["status"] = statusErr.StatusCode
		if statusErr.Hint != "" {
			details["hint"] = statusErr.Hint
		}
	}

	var transportErr *igwerr.TransportError
	if errors.As(err, &transportErr) {
		if transportErr.Timeout {
			details["timeout"] = true
		}
	}

	if len(details) > 0 {
		payload["details"] = details
	}

	return payload
}
