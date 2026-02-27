package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/exitcode"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type callBatchDefaults struct {
	Retry        int
	RetryBackoff time.Duration
	Timeout      time.Duration
	Yes          bool
	SpecFile     string
	Profile      string
	GatewayURL   string
	APIKey       string
	IncludeHeads bool
	OutputFormat string
	Parallel     int
	Compact      bool
}

type callBatchItem struct {
	ID           any      `json:"id,omitempty"`
	OperationID  string   `json:"op,omitempty"`
	Method       string   `json:"method,omitempty"`
	Path         string   `json:"path,omitempty"`
	Query        []string `json:"query,omitempty"`
	Headers      []string `json:"headers,omitempty"`
	Body         string   `json:"body,omitempty"`
	ContentType  string   `json:"contentType,omitempty"`
	DryRun       bool     `json:"dryRun,omitempty"`
	Yes          *bool    `json:"yes,omitempty"`
	Retry        *int     `json:"retry,omitempty"`
	RetryBackoff string   `json:"retryBackoff,omitempty"`
	Timeout      string   `json:"timeout,omitempty"`
}

type callBatchItemResult struct {
	Index    int              `json:"-"`
	ID       any              `json:"id,omitempty"`
	OK       bool             `json:"ok"`
	Code     int              `json:"code"`
	Status   int              `json:"status,omitempty"`
	Error    string           `json:"error,omitempty"`
	TimingMs int64            `json:"timingMs"`
	Request  callJSONRequest  `json:"request,omitempty"`
	Response callJSONResponse `json:"response,omitempty"`
	Stats    *callStats       `json:"stats,omitempty"`
}

type batchExitError struct {
	msg  string
	code int
}

func (e *batchExitError) Error() string {
	return e.msg
}

func (e *batchExitError) ExitCode() int {
	return e.code
}

func (c *CLI) runCallBatch(baseURL string, token string, inputSource string, defaults callBatchDefaults) error {
	if defaults.Parallel <= 0 {
		return &igwerr.UsageError{Msg: "--parallel must be >= 1"}
	}
	if defaults.Timeout <= 0 {
		return &igwerr.UsageError{Msg: "--timeout must be positive"}
	}
	if defaults.Retry < 0 {
		return &igwerr.UsageError{Msg: "--retry must be >= 0"}
	}
	if defaults.Retry > 0 && defaults.RetryBackoff <= 0 {
		return &igwerr.UsageError{Msg: "--retry-backoff must be positive when --retry is set"}
	}

	format := strings.ToLower(strings.TrimSpace(defaults.OutputFormat))
	if format == "" {
		format = "ndjson"
	}
	if format != "ndjson" && format != "json" {
		return &igwerr.UsageError{Msg: "--batch-output must be one of: ndjson, json"}
	}

	items, err := readCallBatchItems(c.In, inputSource)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return &igwerr.UsageError{Msg: "batch request list is empty"}
	}

	opMap, err := c.loadBatchOperationMap(items, defaults)
	if err != nil {
		return err
	}

	client := &gateway.Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    c.runtimeHTTPClient(),
	}

	results := make([]callBatchItemResult, len(items))
	if defaults.Parallel == 1 {
		for i, item := range items {
			results[i] = c.executeBatchCallItem(client, i, item, defaults, opMap)
		}
	} else {
		work := make(chan int)
		var wg sync.WaitGroup

		for worker := 0; worker < defaults.Parallel; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range work {
					results[idx] = c.executeBatchCallItem(client, idx, items[idx], defaults, opMap)
				}
			}()
		}

		for i := range items {
			work <- i
		}
		close(work)
		wg.Wait()
	}

	if writeErr := writeBatchResults(c.Out, results, format, defaults.Compact); writeErr != nil {
		return igwerr.NewTransportError(writeErr)
	}

	exit := aggregateBatchExitCode(results)
	if exit == exitcode.Success {
		return nil
	}
	return &batchExitError{
		msg:  "one or more batch requests failed",
		code: exit,
	}
}

func (c *CLI) executeBatchCallItem(
	client *gateway.Client,
	index int,
	item callBatchItem,
	defaults callBatchDefaults,
	opMap map[string]apidocs.Operation,
) callBatchItemResult {
	out := callBatchItemResult{
		Index: index,
		ID:    item.ID,
	}
	if out.ID == nil {
		out.ID = fmt.Sprintf("%d", index+1)
	}

	input, parseErr := buildCallExecutionInputFromItem(item, callItemExecutionDefaults{
		Timeout:      defaults.Timeout,
		Retry:        defaults.Retry,
		RetryBackoff: defaults.RetryBackoff,
		Yes:          defaults.Yes,
		OperationMap: opMap,
		EnableTiming: true,
	})
	if parseErr != nil {
		out.OK = false
		out.Code = exitCodeForError(parseErr)
		out.Error = "batch item: " + parseErr.Error()
		return out
	}

	start := time.Now()
	resp, reqMethod, reqPath, err := executeCallCore(client, input)
	out.TimingMs = time.Since(start).Milliseconds()
	if reqMethod != "" || reqPath != "" {
		out.Request = callJSONRequest{Method: reqMethod, URL: reqPath}
	}
	if err != nil {
		if _, ok := err.(*igwerr.UsageError); ok {
			err = &igwerr.UsageError{Msg: "batch item: " + err.Error()}
		}
		out.OK = false
		out.Code = exitCodeForError(err)
		out.Error = err.Error()
		stats := buildCallStats(resp, out.TimingMs)
		out.Stats = &stats
		return out
	}

	out.OK = true
	out.Code = exitcode.Success
	out.Status = resp.StatusCode
	out.Request = callJSONRequest{
		Method: resp.Method,
		URL:    resp.URL,
	}
	out.Response = callJSONResponse{
		Status:    resp.StatusCode,
		Headers:   maybeHeaders(resp.Headers, defaults.IncludeHeads),
		Body:      string(resp.Body),
		Bytes:     resp.BodyBytes,
		Truncated: resp.Truncated,
	}
	stats := buildCallStats(resp, out.TimingMs)
	out.Stats = &stats
	return out
}

func (c *CLI) loadBatchOperationMap(items []callBatchItem, defaults callBatchDefaults) (map[string]apidocs.Operation, error) {
	needsOps := false
	for _, item := range items {
		if strings.TrimSpace(item.OperationID) != "" {
			needsOps = true
			break
		}
	}
	if !needsOps {
		return nil, nil
	}

	ops, err := c.loadAPIOperations(defaults.SpecFile, apiSyncRuntime{
		Profile:    defaults.Profile,
		GatewayURL: defaults.GatewayURL,
		APIKey:     defaults.APIKey,
		Timeout:    defaults.Timeout,
	})
	if err != nil {
		return nil, err
	}

	out := make(map[string]apidocs.Operation, len(ops))
	for _, op := range ops {
		opID := strings.TrimSpace(op.OperationID)
		if opID == "" {
			continue
		}
		if _, exists := out[opID]; !exists {
			out[opID] = op
		}
		lower := strings.ToLower(opID)
		if _, exists := out[lower]; !exists {
			out[lower] = op
		}
	}
	return out, nil
}

func readCallBatchItems(stdin io.Reader, source string) ([]callBatchItem, error) {
	reader, closer, err := readBatchSource(stdin, source)
	if err != nil {
		return nil, err
	}
	defer closer()

	buffered := bufio.NewReader(reader)
	leadByte, isEOF := peekBatchLeadByte(buffered)
	if isEOF {
		return nil, &igwerr.UsageError{Msg: "batch input is empty"}
	}
	if leadByte == '[' {
		return decodeBatchJSONArray(buffered)
	}
	return decodeBatchNDJSON(buffered)
}

func peekBatchLeadByte(reader *bufio.Reader) (byte, bool) {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, true
		}
		if !isJSONWhitespace(b) {
			if err := reader.UnreadByte(); err != nil {
				return 0, true
			}
			return b, false
		}
	}
}

func decodeBatchNDJSON(reader *bufio.Reader) ([]callBatchItem, error) {
	scanner := bufio.NewScanner(reader)
	// Allow large NDJSON payload lines for agent workflows.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	items := make([]callBatchItem, 0, 16)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var item callBatchItem
		if err := json.Unmarshal([]byte(text), &item); err != nil {
			return nil, &igwerr.UsageError{Msg: fmt.Sprintf("parse batch NDJSON line %d: %v", line, err)}
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, igwerr.NewTransportError(err)
	}
	return items, nil
}

func decodeBatchJSONArray(reader *bufio.Reader) ([]callBatchItem, error) {
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return nil, &igwerr.UsageError{Msg: fmt.Sprintf("parse batch JSON array: %v", err)}
	}
	openDelim, ok := token.(json.Delim)
	if !ok || openDelim != '[' {
		return nil, &igwerr.UsageError{Msg: "parse batch JSON array: expected '['"}
	}

	items := make([]callBatchItem, 0, 16)
	for decoder.More() {
		var item callBatchItem
		if err := decoder.Decode(&item); err != nil {
			return nil, &igwerr.UsageError{Msg: fmt.Sprintf("parse batch JSON array item: %v", err)}
		}
		items = append(items, item)
	}
	closeToken, err := decoder.Token()
	if err != nil {
		return nil, &igwerr.UsageError{Msg: fmt.Sprintf("parse batch JSON array: %v", err)}
	}
	if closeDelim, ok := closeToken.(json.Delim); !ok || closeDelim != ']' {
		return nil, &igwerr.UsageError{Msg: "parse batch JSON array: expected ']'"}
	}
	if len(items) == 0 {
		return nil, &igwerr.UsageError{Msg: "batch input is empty"}
	}
	return items, nil
}

func readBatchSource(stdin io.Reader, source string) (io.Reader, func() error, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, func() error { return nil }, &igwerr.UsageError{Msg: "required: --batch"}
	}
	source = strings.TrimPrefix(source, "@")
	if source == "-" {
		return stdin, func() error { return nil }, nil
	}

	file, err := os.Open(strings.TrimSpace(source))
	if err != nil {
		return nil, func() error { return nil }, &igwerr.UsageError{Msg: fmt.Sprintf("read batch file: %v", err)}
	}
	return file, file.Close, nil
}

func isJSONWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func writeBatchResults(w io.Writer, items []callBatchItemResult, format string, compact bool) error {
	if format == "json" {
		return writeJSONWithOptions(w, items, compact)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for i := range items {
		item := items[i]
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func aggregateBatchExitCode(results []callBatchItemResult) int {
	if len(results) == 0 {
		return exitcode.Success
	}

	hasUsage := false
	hasAuth := false
	hasNetwork := false

	for _, item := range results {
		switch item.Code {
		case exitcode.Success:
		case exitcode.Usage:
			hasUsage = true
		case exitcode.Auth:
			hasAuth = true
		default:
			hasNetwork = true
		}
	}

	switch {
	case hasUsage:
		return exitcode.Usage
	case hasNetwork:
		return exitcode.Network
	case hasAuth:
		return exitcode.Auth
	default:
		return exitcode.Success
	}
}

func exitCodeForError(err error) int {
	if err == nil {
		return exitcode.Success
	}
	var batchErr interface{ ExitCode() int }
	if errors.As(err, &batchErr) {
		return batchErr.ExitCode()
	}
	return igwerr.ExitCode(err)
}
