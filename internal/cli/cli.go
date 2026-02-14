package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
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
	}
}

func (c *CLI) Execute(args []string) error {
	if len(args) == 0 {
		c.printRootUsage()
		return &igwerr.UsageError{Msg: "required command"}
	}

	switch args[0] {
	case "call":
		return c.runCall(args[1:])
	case "api":
		return c.runAPI(args[1:])
	case "completion":
		return c.runCompletion(args[1:])
	case "config":
		return c.runConfig(args[1:])
	case "doctor":
		return c.runDoctor(args[1:])
	case "gateway":
		return c.runGateway(args[1:])
	case "scan":
		return c.runScan(args[1:])
	case "help", "-h", "--help":
		c.printRootUsage()
		return nil
	default:
		c.printRootUsage()
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown command %q", args[0])}
	}
}

func (c *CLI) runCall(args []string) error {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var (
		gatewayURL     string
		apiKey         string
		apiKeyStdin    bool
		profile        string
		op             string
		specFile       string
		method         string
		path           string
		body           string
		contentType    string
		dryRun         bool
		yes            bool
		retry          int
		retryBackoff   time.Duration
		outPath        string
		timeout        time.Duration
		jsonOutput     bool
		includeHeaders bool
		queries        stringList
		headers        stringList
	)

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.StringVar(&op, "op", "", "OpenAPI operationId to call")
	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file (used with --op)")
	fs.StringVar(&method, "method", "", "HTTP method")
	fs.StringVar(&path, "path", "", "API path")
	fs.Var(&queries, "query", "Query parameter key=value (repeatable)")
	fs.Var(&headers, "header", "Request header key:value (repeatable)")
	fs.StringVar(&body, "body", "", "Request body, @file, or - for stdin")
	fs.StringVar(&contentType, "content-type", "", "Content-Type header value")
	fs.BoolVar(&dryRun, "dry-run", false, "Append dryRun=true query parameter")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating requests (POST/PUT/PATCH/DELETE)")
	fs.IntVar(&retry, "retry", 0, "Retry attempts for idempotent requests")
	fs.DurationVar(&retryBackoff, "retry-backoff", 250*time.Millisecond, "Retry backoff duration")
	fs.StringVar(&outPath, "out", "", "Write response body to file")
	fs.DurationVar(&timeout, "timeout", 8*time.Second, "Request timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&includeHeaders, "include-headers", false, "Include response headers in output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}

	if fs.NArg() > 0 {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	if apiKeyStdin {
		if apiKey != "" {
			return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printCallError(jsonOutput, igwerr.NewTransportError(err))
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	resolved, err := c.resolveRuntimeConfig(profile, gatewayURL, apiKey)
	if err != nil {
		return c.printCallError(jsonOutput, err)
	}

	if strings.TrimSpace(op) != "" {
		if strings.TrimSpace(method) != "" || strings.TrimSpace(path) != "" {
			return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "use either --op or --method/--path, not both"})
		}

		ops, loadErr := loadAPIOperations(specFile)
		if loadErr != nil {
			return c.printCallError(jsonOutput, loadErr)
		}

		matches := apidocs.FilterByOperationID(ops, op)
		if len(matches) == 0 {
			return c.printCallError(jsonOutput, &igwerr.UsageError{
				Msg: fmt.Sprintf("operationId %q not found in spec %q", strings.TrimSpace(op), strings.TrimSpace(specFile)),
			})
		}
		if len(matches) > 1 {
			return c.printCallError(jsonOutput, &igwerr.UsageError{
				Msg: fmt.Sprintf("operationId %q is ambiguous (%d matches): %s", strings.TrimSpace(op), len(matches), formatOperationMatches(matches)),
			})
		}

		method = matches[0].Method
		path = matches[0].Path
	}

	if strings.TrimSpace(resolved.GatewayURL) == "" {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"})
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"})
	}
	if strings.TrimSpace(method) == "" {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "required: --method"})
	}
	if strings.TrimSpace(path) == "" {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "required: --path"})
	}
	if timeout <= 0 {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "--timeout must be positive"})
	}
	if retry < 0 {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "--retry must be >= 0"})
	}
	if retry > 0 && retryBackoff <= 0 {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: "--retry-backoff must be positive when --retry is set"})
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)

	if dryRun {
		queries = append(queries, "dryRun=true")
	}
	if isMutatingMethod(method) && !yes {
		return c.printCallError(jsonOutput, &igwerr.UsageError{
			Msg: fmt.Sprintf("method %s requires --yes confirmation", method),
		})
	}
	if retry > 0 && !isIdempotentMethod(method) {
		return c.printCallError(jsonOutput, &igwerr.UsageError{
			Msg: fmt.Sprintf("--retry is only supported for idempotent methods; got %s", method),
		})
	}

	bodyBytes, err := readBody(c.In, body)
	if err != nil {
		return c.printCallError(jsonOutput, err)
	}

	if len(bodyBytes) > 0 && strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.HTTPClient,
	}

	resp, err := client.Call(context.Background(), gateway.CallRequest{
		Method:       method,
		Path:         path,
		Query:        queries,
		Headers:      headers,
		Body:         bodyBytes,
		ContentType:  contentType,
		Timeout:      timeout,
		Retry:        retry,
		RetryBackoff: retryBackoff,
	})
	if err != nil {
		return c.printCallError(jsonOutput, err)
	}

	bodyFile := ""
	if strings.TrimSpace(outPath) != "" {
		if writeErr := os.WriteFile(outPath, resp.Body, 0o600); writeErr != nil {
			return c.printCallError(jsonOutput, igwerr.NewTransportError(writeErr))
		}
		bodyFile = outPath
	}

	if jsonOutput {
		return writeCallJSON(c.Out, callJSONEnvelope{
			OK: true,
			Request: callJSONRequest{
				Method: resp.Method,
				URL:    resp.URL,
			},
			Response: callJSONResponse{
				Status:   resp.StatusCode,
				Headers:  maybeHeaders(resp.Headers, includeHeaders),
				Body:     string(resp.Body),
				BodyFile: bodyFile,
			},
		})
	}

	if includeHeaders {
		fmt.Fprintf(c.Out, "HTTP %d\n", resp.StatusCode)
		for k, vals := range resp.Headers {
			for _, v := range vals {
				fmt.Fprintf(c.Out, "%s: %s\n", k, v)
			}
		}
		fmt.Fprintln(c.Out)
	}

	if bodyFile != "" {
		fmt.Fprintf(c.Out, "saved response body: %s\n", bodyFile)
		return nil
	}

	if len(resp.Body) > 0 {
		if _, err := c.Out.Write(resp.Body); err != nil {
			return igwerr.NewTransportError(err)
		}
	}

	return nil
}

func (c *CLI) printCallError(jsonOutput bool, err error) error {
	if jsonOutput {
		var statusErr *igwerr.StatusError
		details := map[string]any{}
		if errors.As(err, &statusErr) {
			details["status"] = statusErr.StatusCode
			if statusErr.Hint != "" {
				details["hint"] = statusErr.Hint
			}
		}

		_ = writeCallJSON(c.Out, callJSONEnvelope{
			OK:    false,
			Code:  igwerr.ExitCode(err),
			Error: err.Error(),
			Details: func() map[string]any {
				if len(details) == 0 {
					return nil
				}
				return details
			}(),
		})
	} else {
		fmt.Fprintln(c.Err, err.Error())
	}

	return err
}

func readBody(stdin io.Reader, input string) ([]byte, error) {
	switch {
	case input == "":
		return nil, nil
	case input == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, igwerr.NewTransportError(err)
		}
		return b, nil
	case strings.HasPrefix(input, "@"):
		path := strings.TrimPrefix(input, "@")
		b, err := os.ReadFile(path) //nolint:gosec // user-selected file path
		if err != nil {
			return nil, &igwerr.UsageError{Msg: fmt.Sprintf("read body file: %v", err)}
		}
		return b, nil
	default:
		return []byte(input), nil
	}
}

type callJSONEnvelope struct {
	OK       bool             `json:"ok"`
	Code     int              `json:"code,omitempty"`
	Error    string           `json:"error,omitempty"`
	Details  map[string]any   `json:"details,omitempty"`
	Request  callJSONRequest  `json:"request,omitempty"`
	Response callJSONResponse `json:"response,omitempty"`
}

type callJSONRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

type callJSONResponse struct {
	Status   int                 `json:"status"`
	Headers  map[string][]string `json:"headers,omitempty"`
	Body     string              `json:"body"`
	BodyFile string              `json:"bodyFile,omitempty"`
}

func maybeHeaders(headers http.Header, include bool) map[string][]string {
	if !include {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for k, values := range headers {
		cp := make([]string, len(values))
		copy(cp, values)
		out[k] = cp
	}

	return out
}

func writeCallJSON(w io.Writer, payload callJSONEnvelope) error {
	return writeJSON(w, payload)
}

func writeJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return igwerr.NewTransportError(err)
	}
	return nil
}

func (c *CLI) printRootUsage() {
	fmt.Fprintln(c.Err, "Usage: igw <command> [flags]")
	fmt.Fprintln(c.Err, "")
	fmt.Fprintln(c.Err, "Commands:")
	fmt.Fprintln(c.Err, "  api    Query local OpenAPI documentation")
	fmt.Fprintln(c.Err, "  call   Execute generic Ignition Gateway API request")
	fmt.Fprintln(c.Err, "  completion Output shell completion script")
	fmt.Fprintln(c.Err, "  config Manage local configuration")
	fmt.Fprintln(c.Err, "  doctor Check connectivity and auth")
	fmt.Fprintln(c.Err, "  gateway Convenience gateway commands")
	fmt.Fprintln(c.Err, "  scan   Convenience scan commands")
}

func (c *CLI) runConfig(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw config <set|show|profile> [flags]")
		return &igwerr.UsageError{Msg: "required config subcommand"}
	}

	switch args[0] {
	case "set":
		return c.runConfigSet(args[1:])
	case "show":
		return c.runConfigShow(args[1:])
	case "profile":
		return c.runConfigProfile(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown config subcommand %q", args[0])}
	}
}

func (c *CLI) runGateway(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw gateway <info> [flags]")
		return &igwerr.UsageError{Msg: "required gateway subcommand"}
	}

	switch args[0] {
	case "info":
		return c.runGatewayInfo(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown gateway subcommand %q", args[0])}
	}
}

func (c *CLI) runGatewayInfo(args []string) error {
	fs := flag.NewFlagSet("gateway info", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var (
		gatewayURL     string
		apiKey         string
		profile        string
		apiKeyStdin    bool
		timeout        time.Duration
		jsonOutput     bool
		includeHeaders bool
		retry          int
		retryBackoff   time.Duration
		outPath        string
	)

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.DurationVar(&timeout, "timeout", 8*time.Second, "Request timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&includeHeaders, "include-headers", false, "Include response headers")
	fs.IntVar(&retry, "retry", 0, "Retry attempts for idempotent requests")
	fs.DurationVar(&retryBackoff, "retry-backoff", 250*time.Millisecond, "Retry backoff duration")
	fs.StringVar(&outPath, "out", "", "Write response body to file")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--timeout", timeout.String(),
		"--retry", fmt.Sprintf("%d", retry),
		"--retry-backoff", retryBackoff.String(),
	}
	if gatewayURL != "" {
		callArgs = append(callArgs, "--gateway-url", gatewayURL)
	}
	if apiKey != "" {
		callArgs = append(callArgs, "--api-key", apiKey)
	}
	if apiKeyStdin {
		callArgs = append(callArgs, "--api-key-stdin")
	}
	if profile != "" {
		callArgs = append(callArgs, "--profile", profile)
	}
	if jsonOutput {
		callArgs = append(callArgs, "--json")
	}
	if includeHeaders {
		callArgs = append(callArgs, "--include-headers")
	}
	if outPath != "" {
		callArgs = append(callArgs, "--out", outPath)
	}

	return c.runCall(callArgs)
}

func (c *CLI) runScan(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw scan <projects> [flags]")
		return &igwerr.UsageError{Msg: "required scan subcommand"}
	}

	switch args[0] {
	case "projects":
		return c.runScanProjects(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown scan subcommand %q", args[0])}
	}
}

func (c *CLI) runScanProjects(args []string) error {
	fs := flag.NewFlagSet("scan projects", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var (
		gatewayURL     string
		apiKey         string
		profile        string
		apiKeyStdin    bool
		timeout        time.Duration
		jsonOutput     bool
		includeHeaders bool
		yes            bool
		dryRun         bool
	)

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.DurationVar(&timeout, "timeout", 8*time.Second, "Request timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&includeHeaders, "include-headers", false, "Include response headers")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")
	fs.BoolVar(&dryRun, "dry-run", false, "Append dryRun=true query parameter")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/scan/projects",
		"--timeout", timeout.String(),
	}
	if gatewayURL != "" {
		callArgs = append(callArgs, "--gateway-url", gatewayURL)
	}
	if apiKey != "" {
		callArgs = append(callArgs, "--api-key", apiKey)
	}
	if apiKeyStdin {
		callArgs = append(callArgs, "--api-key-stdin")
	}
	if profile != "" {
		callArgs = append(callArgs, "--profile", profile)
	}
	if jsonOutput {
		callArgs = append(callArgs, "--json")
	}
	if includeHeaders {
		callArgs = append(callArgs, "--include-headers")
	}
	if dryRun {
		callArgs = append(callArgs, "--dry-run")
	}
	if yes {
		callArgs = append(callArgs, "--yes")
	}

	return c.runCall(callArgs)
}

func (c *CLI) runConfigProfile(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw config profile <add|use|list> [flags]")
		return &igwerr.UsageError{Msg: "required config profile subcommand"}
	}

	switch args[0] {
	case "add":
		return c.runConfigProfileAdd(args[1:])
	case "use":
		return c.runConfigProfileUse(args[1:])
	case "list":
		return c.runConfigProfileList(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown config profile subcommand %q", args[0])}
	}
}

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

func (c *CLI) runAPIList(args []string) error {
	fs := flag.NewFlagSet("api list", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var specFile string
	var method string
	var pathContains string
	var jsonOutput bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&pathContains, "path-contains", "", "Filter by path substring")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	ops, err := loadAPIOperations(specFile)
	if err != nil {
		return err
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
	fs := flag.NewFlagSet("api show", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var specFile string
	var method string
	var path string
	var jsonOutput bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&path, "path", "", "Exact API path to show")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	if strings.TrimSpace(path) == "" {
		return &igwerr.UsageError{Msg: "required: --path"}
	}

	ops, err := loadAPIOperations(specFile)
	if err != nil {
		return err
	}

	ops = apidocs.FilterByPath(ops, strings.TrimSpace(path))
	ops = apidocs.FilterByMethod(ops, method)

	if len(ops) == 0 {
		return &igwerr.UsageError{Msg: fmt.Sprintf("no API operation found for path %q", path)}
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
	fs := flag.NewFlagSet("api search", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var specFile string
	var method string
	var query string
	var jsonOutput bool

	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")
	fs.StringVar(&method, "method", "", "Filter by HTTP method")
	fs.StringVar(&query, "query", "", "Search text")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		if strings.TrimSpace(query) == "" && fs.NArg() == 1 {
			query = fs.Arg(0)
		} else {
			return &igwerr.UsageError{Msg: "unexpected positional arguments"}
		}
	}

	if strings.TrimSpace(query) == "" {
		return &igwerr.UsageError{Msg: "required: --query (or one positional query argument)"}
	}

	ops, err := loadAPIOperations(specFile)
	if err != nil {
		return err
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
	specFile = strings.TrimSpace(specFile)
	if specFile == "" {
		specFile = apidocs.DefaultSpecFile
	}

	ops, err := apidocs.LoadOperations(specFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &igwerr.UsageError{
				Msg: fmt.Sprintf("OpenAPI spec not found at %q (pass --spec-file /path/to/openapi.json)", specFile),
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

func (c *CLI) runConfigSet(args []string) error {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var autoGateway bool
	var profileName string
	var apiKey string
	var apiKeyStdin bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.BoolVar(&autoGateway, "auto-gateway", false, "Detect Windows host IP from WSL and set gateway URL")
	fs.StringVar(&profileName, "profile", "", "Profile to update instead of default config")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	if apiKeyStdin {
		if apiKey != "" {
			return &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"}
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return igwerr.NewTransportError(err)
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	if autoGateway && strings.TrimSpace(gatewayURL) != "" {
		return &igwerr.UsageError{Msg: "use only one of --gateway-url or --auto-gateway"}
	}

	if autoGateway {
		if c.DetectWSLHostIP == nil {
			return &igwerr.UsageError{Msg: "auto-gateway is not available in this runtime"}
		}

		hostIP, source, detectErr := c.DetectWSLHostIP()
		if detectErr != nil {
			return &igwerr.UsageError{Msg: fmt.Sprintf("auto-gateway failed: %v", detectErr)}
		}

		gatewayURL = fmt.Sprintf("http://%s:8088", hostIP)
		fmt.Fprintf(c.Out, "auto-detected gateway URL from %s: %s\n", source, gatewayURL)
	}

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	profileName = strings.TrimSpace(profileName)
	if profileName != "" {
		if cfg.Profiles == nil {
			cfg.Profiles = map[string]config.Profile{}
		}

		profileCfg := cfg.Profiles[profileName]
		if strings.TrimSpace(gatewayURL) != "" {
			profileCfg.GatewayURL = strings.TrimSpace(gatewayURL)
		}
		if strings.TrimSpace(apiKey) != "" {
			profileCfg.Token = strings.TrimSpace(apiKey)
		}
		cfg.Profiles[profileName] = profileCfg
	} else {
		if strings.TrimSpace(gatewayURL) != "" {
			cfg.GatewayURL = strings.TrimSpace(gatewayURL)
		}
		if strings.TrimSpace(apiKey) != "" {
			cfg.Token = strings.TrimSpace(apiKey)
		}
	}

	if c.WriteConfig == nil {
		return &igwerr.UsageError{Msg: "config writer is not configured"}
	}
	if err := c.WriteConfig(cfg); err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)}
	}

	path, pathErr := config.Path()
	if pathErr == nil {
		fmt.Fprintf(c.Out, "saved config: %s\n", path)
	} else {
		fmt.Fprintln(c.Out, "saved config")
	}
	if profileName != "" {
		fmt.Fprintf(c.Out, "updated profile: %s\n", profileName)
	}

	return nil
}

func (c *CLI) runConfigShow(args []string) error {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	if jsonOutput {
		type profileView struct {
			GatewayURL  string `json:"gatewayURL,omitempty"`
			TokenMasked string `json:"tokenMasked,omitempty"`
		}
		profiles := map[string]profileView{}
		for name, profile := range cfg.Profiles {
			profiles[name] = profileView{
				GatewayURL:  profile.GatewayURL,
				TokenMasked: config.MaskToken(profile.Token),
			}
		}
		payload := map[string]any{
			"gatewayURL":    cfg.GatewayURL,
			"tokenMasked":   config.MaskToken(cfg.Token),
			"activeProfile": cfg.ActiveProfile,
			"profiles":      profiles,
			"profileCount":  len(profiles),
		}
		enc := json.NewEncoder(c.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return igwerr.NewTransportError(err)
		}
		return nil
	}

	fmt.Fprintf(c.Out, "gateway_url\t%s\n", cfg.GatewayURL)
	fmt.Fprintf(c.Out, "token\t%s\n", config.MaskToken(cfg.Token))
	if strings.TrimSpace(cfg.ActiveProfile) != "" {
		fmt.Fprintf(c.Out, "active_profile\t%s\n", cfg.ActiveProfile)
	}
	if len(cfg.Profiles) > 0 {
		for name, profile := range cfg.Profiles {
			fmt.Fprintf(c.Out, "profile\t%s\t%s\t%s\n", name, profile.GatewayURL, config.MaskToken(profile.Token))
		}
	}
	return nil
}

func (c *CLI) runConfigProfileAdd(args []string) error {
	if len(args) == 0 {
		return &igwerr.UsageError{Msg: "usage: igw config profile add <name> [flags]"}
	}
	name := strings.TrimSpace(args[0])
	if strings.HasPrefix(name, "-") || name == "" {
		return &igwerr.UsageError{Msg: "usage: igw config profile add <name> [flags]"}
	}

	fs := flag.NewFlagSet("config profile add", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var autoGateway bool
	var apiKey string
	var apiKeyStdin bool
	var makeActive bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.BoolVar(&autoGateway, "auto-gateway", false, "Detect Windows host IP from WSL and set gateway URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.BoolVar(&makeActive, "use", false, "Set added profile as active profile")

	if err := fs.Parse(args[1:]); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	if apiKeyStdin {
		if apiKey != "" {
			return &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"}
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return igwerr.NewTransportError(err)
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	if autoGateway && strings.TrimSpace(gatewayURL) != "" {
		return &igwerr.UsageError{Msg: "use only one of --gateway-url or --auto-gateway"}
	}
	if autoGateway {
		if c.DetectWSLHostIP == nil {
			return &igwerr.UsageError{Msg: "auto-gateway is not available in this runtime"}
		}
		hostIP, _, detectErr := c.DetectWSLHostIP()
		if detectErr != nil {
			return &igwerr.UsageError{Msg: fmt.Sprintf("auto-gateway failed: %v", detectErr)}
		}
		gatewayURL = fmt.Sprintf("http://%s:8088", hostIP)
	}

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Profile{}
	}

	profile := cfg.Profiles[name]
	if strings.TrimSpace(gatewayURL) != "" {
		profile.GatewayURL = strings.TrimSpace(gatewayURL)
	}
	if strings.TrimSpace(apiKey) != "" {
		profile.Token = strings.TrimSpace(apiKey)
	}
	cfg.Profiles[name] = profile

	if makeActive {
		cfg.ActiveProfile = name
	}

	if c.WriteConfig == nil {
		return &igwerr.UsageError{Msg: "config writer is not configured"}
	}
	if err := c.WriteConfig(cfg); err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)}
	}

	fmt.Fprintf(c.Out, "saved profile: %s\n", name)
	if makeActive {
		fmt.Fprintf(c.Out, "active profile: %s\n", name)
	}
	return nil
}

func (c *CLI) runConfigProfileUse(args []string) error {
	if len(args) == 0 {
		return &igwerr.UsageError{Msg: "usage: igw config profile use <name>"}
	}
	name := strings.TrimSpace(args[0])
	if strings.HasPrefix(name, "-") || name == "" {
		return &igwerr.UsageError{Msg: "usage: igw config profile use <name>"}
	}

	fs := flag.NewFlagSet("config profile use", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	if err := fs.Parse(args[1:]); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return &igwerr.UsageError{Msg: fmt.Sprintf("profile %q not found", name)}
	}
	cfg.ActiveProfile = name

	if c.WriteConfig == nil {
		return &igwerr.UsageError{Msg: "config writer is not configured"}
	}
	if err := c.WriteConfig(cfg); err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)}
	}

	fmt.Fprintf(c.Out, "active profile: %s\n", name)
	return nil
}

func (c *CLI) runConfigProfileList(args []string) error {
	fs := flag.NewFlagSet("config profile list", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	type profileView struct {
		Name        string `json:"name"`
		Active      bool   `json:"active"`
		GatewayURL  string `json:"gatewayURL,omitempty"`
		TokenMasked string `json:"tokenMasked,omitempty"`
	}

	views := make([]profileView, 0, len(cfg.Profiles))
	for name, profile := range cfg.Profiles {
		views = append(views, profileView{
			Name:        name,
			Active:      name == cfg.ActiveProfile,
			GatewayURL:  profile.GatewayURL,
			TokenMasked: config.MaskToken(profile.Token),
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Name < views[j].Name
	})

	if jsonOutput {
		return writeJSON(c.Out, map[string]any{
			"activeProfile": cfg.ActiveProfile,
			"count":         len(views),
			"profiles":      views,
		})
	}

	fmt.Fprintln(c.Out, "ACTIVE\tNAME\tGATEWAY_URL\tTOKEN")
	for _, view := range views {
		active := ""
		if view.Active {
			active = "*"
		}
		fmt.Fprintf(c.Out, "%s\t%s\t%s\t%s\n", active, view.Name, view.GatewayURL, view.TokenMasked)
	}

	return nil
}

func (c *CLI) runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var apiKey string
	var apiKeyStdin bool
	var profile string
	var timeout time.Duration
	var jsonOutput bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.DurationVar(&timeout, "timeout", 5*time.Second, "Check timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	if apiKeyStdin {
		if apiKey != "" {
			return &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"}
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return igwerr.NewTransportError(err)
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	resolved, err := c.resolveRuntimeConfig(profile, gatewayURL, apiKey)
	if err != nil {
		return err
	}

	if strings.TrimSpace(resolved.GatewayURL) == "" {
		return &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"}
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"}
	}
	if timeout <= 0 {
		return &igwerr.UsageError{Msg: "--timeout must be positive"}
	}

	checks := make([]doctorCheck, 0, 4)

	parsedURL, err := url.Parse(resolved.GatewayURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		uerr := &igwerr.UsageError{Msg: "invalid gateway URL"}
		checks = append(checks, doctorCheck{
			Name:    "gateway_url",
			OK:      false,
			Message: uerr.Error(),
			Hint:    "Use a full URL like http://<windows-host-ip>:8088",
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, uerr)
	}
	checks = append(checks, doctorCheck{
		Name:    "gateway_url",
		OK:      true,
		Message: "parsed",
	})

	addr, addrErr := dialAddress(parsedURL)
	if addrErr != nil {
		uerr := &igwerr.UsageError{Msg: addrErr.Error()}
		checks = append(checks, doctorCheck{
			Name:    "tcp_connect",
			OK:      false,
			Message: uerr.Error(),
			Hint:    "Gateway URL must include a valid host and scheme",
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, uerr)
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		nerr := igwerr.NewTransportError(err)
		checks = append(checks, doctorCheck{
			Name:    "tcp_connect",
			OK:      false,
			Message: nerr.Error(),
			Hint:    doctorHintForError(nerr),
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, nerr)
	}
	_ = conn.Close()
	checks = append(checks, doctorCheck{
		Name:    "tcp_connect",
		OK:      true,
		Message: addr,
	})

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.HTTPClient,
	}
	resp, err := client.Call(context.Background(), gateway.CallRequest{
		Method:  http.MethodGet,
		Path:    "/data/api/v1/gateway-info",
		Timeout: timeout,
	})
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "gateway_info",
			OK:      false,
			Message: err.Error(),
			Hint:    doctorHintForError(err),
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, err)
	}

	checks = append(checks, doctorCheck{
		Name:    "gateway_info",
		OK:      true,
		Message: fmt.Sprintf("status %d", resp.StatusCode),
	})

	writeResp, err := client.Call(context.Background(), gateway.CallRequest{
		Method:  http.MethodPost,
		Path:    "/data/api/v1/scan/projects",
		Timeout: timeout,
	})
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "scan_projects",
			OK:      false,
			Message: err.Error(),
			Hint:    doctorHintForError(err),
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, err)
	}
	checks = append(checks, doctorCheck{
		Name:    "scan_projects",
		OK:      true,
		Message: fmt.Sprintf("status %d", writeResp.StatusCode),
	})

	return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, nil)
}

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type doctorEnvelope struct {
	OK         bool          `json:"ok"`
	Code       int           `json:"code,omitempty"`
	Error      string        `json:"error,omitempty"`
	GatewayURL string        `json:"gatewayURL"`
	Checks     []doctorCheck `json:"checks"`
}

func (c *CLI) printDoctorResult(jsonOutput bool, gatewayURL string, checks []doctorCheck, err error) error {
	if jsonOutput {
		payload := doctorEnvelope{
			OK:         err == nil,
			GatewayURL: gatewayURL,
			Checks:     checks,
		}
		if err != nil {
			payload.Code = igwerr.ExitCode(err)
			payload.Error = err.Error()
		}

		enc := json.NewEncoder(c.Out)
		enc.SetIndent("", "  ")
		if encodeErr := enc.Encode(payload); encodeErr != nil {
			return igwerr.NewTransportError(encodeErr)
		}
		return err
	}

	for _, check := range checks {
		state := "ok"
		if !check.OK {
			state = "fail"
		}
		if check.Hint != "" {
			fmt.Fprintf(c.Out, "%s\t%s\t%s\thint: %s\n", state, check.Name, check.Message, check.Hint)
			continue
		}
		fmt.Fprintf(c.Out, "%s\t%s\t%s\n", state, check.Name, check.Message)
	}

	if err != nil {
		fmt.Fprintln(c.Err, err.Error())
	}
	return err
}

func doctorHintForError(err error) string {
	var statusErr *igwerr.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized:
			return "401 indicates a missing or invalid token. Re-check your API key."
		case http.StatusForbidden:
			return "403 indicates permission mapping or secure-connection restrictions. Ensure token security levels are included in Gateway Read permissions."
		}
	}

	var transportErr *igwerr.TransportError
	if errors.As(err, &transportErr) && transportErr.Timeout {
		return "If this is WSL2 -> Windows, allow inbound TCP 8088 on interface alias \"vEthernet (WSL (Hyper-V firewall))\"."
	}

	if errors.As(err, &transportErr) {
		return "Check gateway host/port reachability and local firewall rules."
	}

	return ""
}

func (c *CLI) resolveRuntimeConfig(profile string, gatewayURL string, apiKey string) (config.Effective, error) {
	cfg, err := c.ReadConfig()
	if err != nil {
		return config.Effective{}, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	resolved, err := config.ResolveWithProfile(cfg, c.Getenv, gatewayURL, apiKey, profile)
	if err != nil {
		return config.Effective{}, &igwerr.UsageError{Msg: err.Error()}
	}

	return resolved, nil
}

func isMutatingMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isIdempotentMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func bashCompletionScript() string {
	return `# bash completion for igw
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
    config)
      COMPREPLY=( $(compgen -W "set show profile" -- "${cur}") )
      return 0
      ;;
    gateway)
      COMPREPLY=( $(compgen -W "info" -- "${cur}") )
      return 0
      ;;
    scan)
      COMPREPLY=( $(compgen -W "projects" -- "${cur}") )
      return 0
      ;;
    api)
      COMPREPLY=( $(compgen -W "list show search" -- "${cur}") )
      return 0
      ;;
    profile)
      if [[ "${cmd1}" == "config" ]]; then
        COMPREPLY=( $(compgen -W "add use list" -- "${cur}") )
        return 0
      fi
      ;;
  esac

  if [[ ${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "api call completion config doctor gateway help scan" -- "${cur}") )
    return 0
  fi

  if [[ "${cmd1}" == "config" && ${COMP_CWORD} -eq 2 ]]; then
    COMPREPLY=( $(compgen -W "set show profile" -- "${cur}") )
    return 0
  fi

  if [[ "${cmd1}" == "config" && "${cmd2}" == "profile" && ${COMP_CWORD} -eq 3 ]]; then
    COMPREPLY=( $(compgen -W "add use list" -- "${cur}") )
    return 0
  fi

  COMPREPLY=( $(compgen -W "--profile --gateway-url --api-key --api-key-stdin --timeout --json --include-headers --spec-file --op --method --path --query --header --body --content-type --yes --dry-run --retry --retry-backoff --out" -- "${cur}") )
}

complete -F _igw_completion igw
`
}

func dialAddress(gatewayURL *url.URL) (string, error) {
	host := strings.TrimSpace(gatewayURL.Hostname())
	if host == "" {
		return "", fmt.Errorf("gateway URL host is empty")
	}

	port := strings.TrimSpace(gatewayURL.Port())
	if port == "" {
		switch strings.ToLower(gatewayURL.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", fmt.Errorf("unsupported URL scheme %q", gatewayURL.Scheme)
		}
	}

	return net.JoinHostPort(host, port), nil
}
