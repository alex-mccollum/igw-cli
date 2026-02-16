package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runCall(args []string) error {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var (
		common       wrapperCommon
		op           string
		specFile     string
		method       string
		path         string
		body         string
		contentType  string
		dryRun       bool
		yes          bool
		retry        int
		retryBackoff time.Duration
		outPath      string
		fieldPath    string
		queries      stringList
		headers      stringList
	)

	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, true)
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
	fs.StringVar(&fieldPath, "field", "", "Extract one value from JSON output using dot path (requires --json)")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}

	selectOpts, selectErr := newJSONSelectOptions(common.jsonOutput, common.compactJSON, fieldPath, common.fieldsCSV)
	if selectErr != nil {
		return c.printCallError(common.jsonOutput, selectionErrorOptions(selectOpts), selectErr)
	}

	if fs.NArg() > 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	if common.apiKeyStdin {
		if common.apiKey != "" {
			return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printCallError(common.jsonOutput, selectOpts, igwerr.NewTransportError(err))
		}
		common.apiKey = strings.TrimSpace(string(tokenBytes))
	}

	resolved, err := c.resolveRuntimeConfig(common.profile, common.gatewayURL, common.apiKey)
	if err != nil {
		return c.printCallError(common.jsonOutput, selectOpts, err)
	}

	if strings.TrimSpace(op) != "" {
		if strings.TrimSpace(method) != "" || strings.TrimSpace(path) != "" {
			return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "use either --op or --method/--path, not both"})
		}

		ops, loadErr := c.loadAPIOperations(specFile, apiSyncRuntime{
			Profile:    common.profile,
			GatewayURL: common.gatewayURL,
			APIKey:     common.apiKey,
			Timeout:    common.timeout,
		})
		if loadErr != nil {
			return c.printCallError(common.jsonOutput, selectOpts, loadErr)
		}

		matches := apidocs.FilterByOperationID(ops, op)
		if len(matches) == 0 {
			return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{
				Msg: fmt.Sprintf("operationId %q not found in spec %q", strings.TrimSpace(op), strings.TrimSpace(specFile)),
			})
		}
		if len(matches) > 1 {
			return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{
				Msg: fmt.Sprintf("operationId %q is ambiguous (%d matches): %s", strings.TrimSpace(op), len(matches), formatOperationMatches(matches)),
			})
		}

		method = matches[0].Method
		path = matches[0].Path
	}

	if strings.TrimSpace(resolved.GatewayURL) == "" {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"})
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"})
	}
	if strings.TrimSpace(path) == "" {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required: --path"})
	}
	if strings.TrimSpace(method) == "" {
		method = http.MethodGet
	}
	if common.timeout <= 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--timeout must be positive"})
	}
	if retry < 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--retry must be >= 0"})
	}
	if retry > 0 && retryBackoff <= 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--retry-backoff must be positive when --retry is set"})
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)

	if dryRun {
		queries = append(queries, "dryRun=true")
	}
	if isMutatingMethod(method) && !yes {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{
			Msg: fmt.Sprintf("method %s requires --yes confirmation", method),
		})
	}
	if retry > 0 && !isIdempotentMethod(method) {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{
			Msg: fmt.Sprintf("--retry is only supported for idempotent methods; got %s", method),
		})
	}

	bodyBytes, err := readBody(c.In, body)
	if err != nil {
		return c.printCallError(common.jsonOutput, selectOpts, err)
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
		Timeout:      common.timeout,
		Retry:        retry,
		RetryBackoff: retryBackoff,
	})
	if err != nil {
		return c.printCallError(common.jsonOutput, selectOpts, err)
	}

	bodyFile := ""
	if strings.TrimSpace(outPath) != "" {
		if writeErr := os.WriteFile(outPath, resp.Body, 0o600); writeErr != nil {
			return c.printCallError(common.jsonOutput, selectOpts, igwerr.NewTransportError(writeErr))
		}
		bodyFile = outPath
	}

	if common.jsonOutput {
		payload := callJSONEnvelope{
			OK: true,
			Request: callJSONRequest{
				Method: resp.Method,
				URL:    resp.URL,
			},
			Response: callJSONResponse{
				Status:   resp.StatusCode,
				Headers:  maybeHeaders(resp.Headers, common.includeHeaders),
				Body:     string(resp.Body),
				BodyFile: bodyFile,
			},
		}
		if selectWriteErr := printJSONSelection(c.Out, payload, selectOpts); selectWriteErr != nil {
			return c.printCallError(common.jsonOutput, selectionErrorOptions(selectOpts), selectWriteErr)
		}
		return nil
	}

	if common.includeHeaders {
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

func (c *CLI) printCallError(jsonOutput bool, selectOpts jsonSelectOptions, err error) error {
	if jsonOutput {
		payload := jsonErrorPayload(err)
		if selectErr := printJSONSelection(c.Out, payload, selectOpts); selectErr != nil {
			_ = writeJSONWithOptions(c.Out, jsonErrorPayload(selectErr), selectOpts.compact)
			return selectErr
		}
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
	return writeJSONWithOptions(w, payload, false)
}

func writeJSONWithOptions(w io.Writer, payload any, compact bool) error {
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(payload); err != nil {
		return igwerr.NewTransportError(err)
	}
	return nil
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
