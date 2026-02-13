package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type CLI struct {
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	Getenv      func(string) string
	ReadConfig  func() (config.File, error)
	WriteConfig func(config.File) error
	HTTPClient  *http.Client
}

func New() *CLI {
	return &CLI{
		In:          os.Stdin,
		Out:         os.Stdout,
		Err:         os.Stderr,
		Getenv:      os.Getenv,
		ReadConfig:  config.Read,
		WriteConfig: config.Write,
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
	case "config":
		return c.runConfig(args[1:])
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
		method         string
		path           string
		body           string
		contentType    string
		timeout        time.Duration
		jsonOutput     bool
		includeHeaders bool
		queries        stringList
		headers        stringList
	)

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&method, "method", "", "HTTP method")
	fs.StringVar(&path, "path", "", "API path")
	fs.Var(&queries, "query", "Query parameter key=value (repeatable)")
	fs.Var(&headers, "header", "Request header key:value (repeatable)")
	fs.StringVar(&body, "body", "", "Request body, @file, or - for stdin")
	fs.StringVar(&contentType, "content-type", "", "Content-Type header value")
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

	cfg, err := c.ReadConfig()
	if err != nil {
		return c.printCallError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)})
	}
	resolved := config.Resolve(cfg, c.Getenv, gatewayURL, apiKey)

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
		Method:      strings.ToUpper(strings.TrimSpace(method)),
		Path:        strings.TrimSpace(path),
		Query:       queries,
		Headers:     headers,
		Body:        bodyBytes,
		ContentType: contentType,
		Timeout:     timeout,
	})
	if err != nil {
		return c.printCallError(jsonOutput, err)
	}

	if jsonOutput {
		return writeCallJSON(c.Out, callJSONEnvelope{
			OK: true,
			Request: callJSONRequest{
				Method: resp.Method,
				URL:    resp.URL,
			},
			Response: callJSONResponse{
				Status:  resp.StatusCode,
				Headers: maybeHeaders(resp.Headers, includeHeaders),
				Body:    string(resp.Body),
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
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body"`
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
	fmt.Fprintln(c.Err, "  call   Execute generic Ignition Gateway API request")
	fmt.Fprintln(c.Err, "  config Manage local configuration")
}

func (c *CLI) runConfig(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw config <set|show> [flags]")
		return &igwerr.UsageError{Msg: "required config subcommand"}
	}

	switch args[0] {
	case "set":
		return c.runConfigSet(args[1:])
	case "show":
		return c.runConfigShow(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown config subcommand %q", args[0])}
	}
}

func (c *CLI) runConfigSet(args []string) error {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var apiKey string
	var apiKeyStdin bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
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

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	if strings.TrimSpace(gatewayURL) != "" {
		cfg.GatewayURL = strings.TrimSpace(gatewayURL)
	}
	if strings.TrimSpace(apiKey) != "" {
		cfg.Token = strings.TrimSpace(apiKey)
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
		payload := map[string]string{
			"gatewayURL":  cfg.GatewayURL,
			"tokenMasked": config.MaskToken(cfg.Token),
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
	return nil
}
