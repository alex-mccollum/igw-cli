package cli

import (
	"encoding/json"
	"errors"
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
		common        wrapperCommon
		op            string
		specFile      string
		batchInput    string
		batchOutput   string
		batchParallel int
		method        string
		path          string
		body          string
		contentType   string
		dryRun        bool
		yes           bool
		stream        bool
		maxBodyBytes  int64
		retry         int
		retryBackoff  time.Duration
		outPath       string
		queries       stringList
		headers       stringList
	)

	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, true)
	fs.StringVar(&op, "op", "", "OpenAPI operationId to call")
	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file (used with --op)")
	fs.StringVar(&batchInput, "batch", "", "Batch request source (@file, file, or - for stdin)")
	fs.StringVar(&batchOutput, "batch-output", "ndjson", "Batch output format: ndjson|json")
	fs.IntVar(&batchParallel, "parallel", 1, "Batch parallel worker count (requires --batch)")
	fs.StringVar(&method, "method", "", "HTTP method")
	fs.StringVar(&path, "path", "", "API path")
	fs.Var(&queries, "query", "Query parameter key=value (repeatable)")
	fs.Var(&headers, "header", "Request header key:value (repeatable)")
	fs.StringVar(&body, "body", "", "Request body, @file, or - for stdin")
	fs.StringVar(&contentType, "content-type", "", "Content-Type header value")
	fs.BoolVar(&dryRun, "dry-run", false, "Append dryRun=true query parameter")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating requests (POST/PUT/PATCH/DELETE)")
	fs.BoolVar(&stream, "stream", false, "Stream response body directly (non-JSON mode)")
	fs.Int64Var(&maxBodyBytes, "max-body-bytes", 0, "Maximum response bytes to read/stream (0 = unlimited)")
	fs.IntVar(&retry, "retry", 0, "Retry attempts for idempotent requests")
	fs.DurationVar(&retryBackoff, "retry-backoff", 250*time.Millisecond, "Retry backoff duration")
	fs.StringVar(&outPath, "out", "", "Write response body to file")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}

	selectOpts, selectErr := newJSONSelectOptions(common.jsonOutput, common.compactJSON, common.rawOutput, common.selectors)
	if selectErr != nil {
		return c.printCallError(common.jsonOutput, selectionErrorOptions(selectOpts), selectErr)
	}

	if fs.NArg() > 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	batchRequested := strings.TrimSpace(batchInput) != ""
	if !batchRequested && batchParallel != 1 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--parallel requires --batch"})
	}
	if batchRequested && len(common.selectors) > 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--select is not supported with --batch"})
	}
	if batchRequested && common.rawOutput {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--raw is not supported with --batch"})
	}
	if batchRequested && strings.TrimSpace(outPath) != "" {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--out is not supported with --batch"})
	}
	if batchRequested && stream {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--stream is not supported with --batch"})
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
		if batchRequested {
			return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--op is not supported with --batch (set op per batch item)"})
		}
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

		matches := resolveOperationsByID(ops, op)
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
	if batchRequested {
		defaults := callBatchDefaults{
			Retry:        retry,
			RetryBackoff: retryBackoff,
			Timeout:      common.timeout,
			Yes:          yes,
			SpecFile:     specFile,
			Profile:      common.profile,
			GatewayURL:   common.gatewayURL,
			APIKey:       common.apiKey,
			IncludeHeads: common.includeHeaders,
			OutputFormat: batchOutput,
			Parallel:     batchParallel,
			Compact:      common.compactJSON,
		}
		return c.runCallBatch(resolved.GatewayURL, resolved.Token, batchInput, defaults)
	}
	if strings.TrimSpace(path) == "" {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required: --path"})
	}
	if common.timeout <= 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--timeout must be positive"})
	}
	if maxBodyBytes < 0 {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--max-body-bytes must be >= 0"})
	}
	method = strings.TrimSpace(method)
	path = strings.TrimSpace(path)
	if stream && common.jsonOutput {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--stream is not supported with --json"})
	}
	if stream && common.includeHeaders && strings.TrimSpace(outPath) == "" {
		return c.printCallError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--include-headers with --stream requires --out"})
	}

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.runtimeHTTPClient(),
	}

	streamWriter, closeStreamWriter, err := c.callOutputWriter(outPath, stream, common.jsonOutput)
	if err != nil {
		return c.printCallError(common.jsonOutput, selectOpts, err)
	}
	if closeStreamWriter != nil {
		defer closeStreamWriter()
	}

	bodyBytes, err := readBody(c.In, body)
	if err != nil {
		return c.printCallError(common.jsonOutput, selectOpts, err)
	}

	start := time.Now()
	resp, _, _, err := executeCallCore(client, callExecutionInput{
		Method:       method,
		Path:         path,
		Query:        queries,
		Headers:      headers,
		Body:         bodyBytes,
		ContentType:  contentType,
		DryRun:       dryRun,
		Yes:          yes,
		Timeout:      common.timeout,
		Retry:        retry,
		RetryBackoff: retryBackoff,
		Stream:       streamWriter,
		MaxBodyBytes: maxBodyBytes,
		EnableTiming: common.timing || common.jsonStats,
	})
	if err != nil {
		return c.printCallError(common.jsonOutput, selectOpts, err)
	}

	bodyFile := ""
	if strings.TrimSpace(outPath) != "" && (stream || !common.jsonOutput) {
		bodyFile = outPath
	}

	timingPayload := buildCallStats(resp, time.Since(start).Milliseconds())

	if common.jsonOutput {
		payload := callJSONEnvelope{
			OK: true,
			Request: callJSONRequest{
				Method: resp.Method,
				URL:    resp.URL,
			},
			Response: callJSONResponse{
				Status:    resp.StatusCode,
				Headers:   maybeHeaders(resp.Headers, common.includeHeaders),
				Body:      string(resp.Body),
				BodyFile:  bodyFile,
				Truncated: resp.Truncated,
				Bytes:     resp.BodyBytes,
			},
		}
		if common.jsonStats || common.timing {
			payload.Stats = &timingPayload
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
		if common.timing {
			printTimingSummary(c.Err, timingPayload)
		}
		return nil
	}

	if stream && strings.TrimSpace(outPath) == "" {
		if common.timing {
			printTimingSummary(c.Err, timingPayload)
		}
		return nil
	}

	if len(resp.Body) > 0 {
		if _, err := c.Out.Write(resp.Body); err != nil {
			return igwerr.NewTransportError(err)
		}
	}
	if common.timing {
		printTimingSummary(c.Err, timingPayload)
	}

	return nil
}

func (c *CLI) callOutputWriter(outPath string, stream bool, jsonOutput bool) (io.Writer, func() error, error) {
	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		if stream {
			return c.Out, nil, nil
		}
		return nil, nil, nil
	}

	if !stream && jsonOutput {
		return nil, nil, nil
	}

	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, nil, igwerr.NewTransportError(err)
	}
	return outFile, outFile.Close, nil
}

func resolveOperationsByID(ops []apidocs.Operation, operationID string) []apidocs.Operation {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil
	}

	exact := make([]apidocs.Operation, 0, 2)
	insensitive := make([]apidocs.Operation, 0, 2)
	for _, op := range ops {
		candidate := strings.TrimSpace(op.OperationID)
		if candidate == "" {
			continue
		}
		if candidate == operationID {
			exact = append(exact, op)
			continue
		}
		if strings.EqualFold(candidate, operationID) {
			insensitive = append(insensitive, op)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return insensitive
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
	Stats    *callStats       `json:"stats,omitempty"`
}

type callJSONRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

type callJSONResponse struct {
	Status    int                 `json:"status"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      string              `json:"body"`
	BodyFile  string              `json:"bodyFile,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
	Bytes     int64               `json:"bytes,omitempty"`
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

func exitCodeFromCallError(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface {
		ExitCode() int
	}
	var coded exitCoder
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return igwerr.ExitCode(err)
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
