package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type callExecutionInput struct {
	Context context.Context

	Method       string
	Path         string
	OperationID  string
	OperationMap map[string]apidocs.Operation

	Query       []string
	Headers     []string
	Body        []byte
	ContentType string
	DryRun      bool
	Yes         bool

	Timeout      time.Duration
	Retry        int
	RetryBackoff time.Duration

	Stream       io.Writer
	MaxBodyBytes int64
	EnableTiming bool
}

const callStatsSchemaVersion = 1

func executeCallCore(client *gateway.Client, input callExecutionInput) (*gateway.CallResponse, string, string, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	path := strings.TrimSpace(input.Path)
	op := strings.TrimSpace(input.OperationID)

	if op != "" {
		if method != "" || path != "" {
			return nil, "", "", &igwerr.UsageError{Msg: "use either op or method/path, not both"}
		}
		match, ok := resolveOperationByID(input.OperationMap, op)
		if !ok {
			return nil, "", "", &igwerr.UsageError{Msg: fmt.Sprintf("operationId %q not found", op)}
		}
		method = match.Method
		path = match.Path
	}

	if path == "" {
		return nil, "", "", &igwerr.UsageError{Msg: "required: --path"}
	}
	if method == "" {
		method = http.MethodGet
	}
	if input.Timeout <= 0 {
		return nil, method, path, &igwerr.UsageError{Msg: "--timeout must be positive"}
	}
	if input.Retry < 0 {
		return nil, method, path, &igwerr.UsageError{Msg: "--retry must be >= 0"}
	}
	if input.Retry > 0 && input.RetryBackoff <= 0 {
		return nil, method, path, &igwerr.UsageError{Msg: "--retry-backoff must be positive when --retry is set"}
	}
	if isMutatingMethod(method) && !input.Yes {
		return nil, method, path, &igwerr.UsageError{Msg: fmt.Sprintf("method %s requires --yes confirmation", method)}
	}
	if input.Retry > 0 && !isIdempotentMethod(method) {
		return nil, method, path, &igwerr.UsageError{
			Msg: fmt.Sprintf("--retry is only supported for idempotent methods; got %s", method),
		}
	}

	query := append([]string(nil), input.Query...)
	if input.DryRun {
		query = append(query, "dryRun=true")
	}

	contentType := strings.TrimSpace(input.ContentType)
	if len(input.Body) > 0 && contentType == "" {
		contentType = "application/json"
	}

	callCtx := input.Context
	if callCtx == nil {
		callCtx = context.Background()
	}

	resp, err := client.Call(callCtx, gateway.CallRequest{
		Method:       method,
		Path:         path,
		Query:        query,
		Headers:      input.Headers,
		Body:         input.Body,
		ContentType:  contentType,
		Timeout:      input.Timeout,
		Retry:        input.Retry,
		RetryBackoff: input.RetryBackoff,
		Stream:       input.Stream,
		MaxBodyBytes: input.MaxBodyBytes,
		EnableTiming: input.EnableTiming,
	})
	return resp, method, path, err
}

func resolveOperationByID(opMap map[string]apidocs.Operation, operationID string) (apidocs.Operation, bool) {
	if len(opMap) == 0 || strings.TrimSpace(operationID) == "" {
		return apidocs.Operation{}, false
	}

	if match, ok := opMap[operationID]; ok {
		return match, true
	}

	lower := strings.ToLower(operationID)
	if match, ok := opMap[lower]; ok {
		return match, true
	}

	for id, match := range opMap {
		if strings.EqualFold(id, operationID) {
			return match, true
		}
	}

	return apidocs.Operation{}, false
}

func buildCallStats(resp *gateway.CallResponse, timingMs int64) map[string]any {
	stats := map[string]any{
		"version":  callStatsSchemaVersion,
		"timingMs": timingMs,
		"bodyBytes": int64(0),
	}
	if resp == nil {
		return stats
	}
	stats["bodyBytes"] = resp.BodyBytes
	if resp.Timing != nil {
		stats["http"] = resp.Timing
	}
	if resp.Truncated {
		stats["truncated"] = true
	}
	return stats
}
