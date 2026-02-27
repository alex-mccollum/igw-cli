package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type wrapperCommon struct {
	gatewayURL     string
	apiKey         string
	apiKeyStdin    bool
	profile        string
	timeout        time.Duration
	jsonOutput     bool
	compactJSON    bool
	selectors      stringList
	rawOutput      bool
	includeHeaders bool
	timing         bool
	jsonStats      bool
}

func bindWrapperCommon(fs *flag.FlagSet, common *wrapperCommon) {
	bindWrapperCommonWithDefaults(fs, common, 8*time.Second, true)
}

func bindWrapperCommonWithDefaults(fs *flag.FlagSet, common *wrapperCommon, timeoutDefault time.Duration, includeHeaders bool) {
	fs.StringVar(&common.gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&common.apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&common.apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&common.profile, "profile", "", "Config profile name")
	fs.DurationVar(&common.timeout, "timeout", timeoutDefault, "Request timeout")
	fs.BoolVar(&common.jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&common.compactJSON, "compact", false, "Print compact one-line JSON (requires --json)")
	fs.Var(&common.selectors, "select", "Select JSON path from output (repeatable, requires --json)")
	fs.BoolVar(&common.rawOutput, "raw", false, "Print selected value as plain text (requires --json and exactly one --select)")
	fs.BoolVar(&common.timing, "timing", false, "Include command timing output")
	fs.BoolVar(&common.jsonStats, "json-stats", false, "Include runtime stats in JSON output")
	if includeHeaders {
		fs.BoolVar(&common.includeHeaders, "include-headers", false, "Include response headers")
	}
}

func (w wrapperCommon) callArgs() []string {
	args := []string{
		"--timeout", w.timeout.String(),
	}
	args = append(args, w.callArgsExcludingTimeout()...)
	return args
}

func (w wrapperCommon) callArgsExcludingTimeout() []string {
	args := make([]string, 0, 10)
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
	if w.compactJSON {
		args = append(args, "--compact")
	}
	for _, selector := range w.selectors {
		if strings.TrimSpace(selector) != "" {
			args = append(args, "--select", strings.TrimSpace(selector))
		}
	}
	if w.rawOutput {
		args = append(args, "--raw")
	}
	if w.includeHeaders {
		args = append(args, "--include-headers")
	}
	if w.timing {
		args = append(args, "--timing")
	}
	if w.jsonStats {
		args = append(args, "--json-stats")
	}
	return args
}

func parseWrapperFlagSet(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	return nil
}

func appendQueryArgs(args []string, query []string) []string {
	for _, pair := range query {
		args = append(args, "--query", pair)
	}
	return args
}

func parseOptionalBoolFlag(flagName string, value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", nil
	}
	switch normalized {
	case "true", "false":
		return normalized, nil
	default:
		return "", &igwerr.UsageError{
			Msg: fmt.Sprintf("invalid value for --%s: %q (expected true or false)", flagName, strings.TrimSpace(value)),
		}
	}
}

func parseRequiredEnumFlag(flagName string, value string, allowed []string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", &igwerr.UsageError{Msg: fmt.Sprintf("required: --%s", flagName)}
	}
	for _, allowedValue := range allowed {
		if strings.EqualFold(allowedValue, normalized) {
			return allowedValue, nil
		}
	}
	return "", &igwerr.UsageError{
		Msg: fmt.Sprintf("invalid value for --%s: %q (allowed: %s)", flagName, normalized, strings.Join(allowed, ", ")),
	}
}

func inferTagImportType(path string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".csv":
		return "csv"
	default:
		return ""
	}
}

func chooseDefaultOutPath(explicitOutPath string, defaultFileName string) string {
	if outPath := strings.TrimSpace(explicitOutPath); outPath != "" {
		return outPath
	}
	return defaultFileName
}
