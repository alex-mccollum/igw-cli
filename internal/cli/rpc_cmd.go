package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type rpcRequest struct {
	ID   any             `json:"id,omitempty"`
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

type rpcResponse struct {
	ID     any    `json:"id,omitempty"`
	OK     bool   `json:"ok"`
	Code   int    `json:"code"`
	Status int    `json:"status,omitempty"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (c *CLI) runRPC(args []string) error {
	fs := flag.NewFlagSet("rpc", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var specFile string
	var workers int
	var queueSize int
	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, false)
	fs.StringVar(&specFile, "spec-file", "openapi.json", "Path to OpenAPI JSON file (for op-based call resolution)")
	fs.IntVar(&workers, "workers", 1, "Number of concurrent request workers")
	fs.IntVar(&queueSize, "queue-size", 64, "RPC request queue capacity")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	if common.apiKeyStdin {
		return &igwerr.UsageError{Msg: "--api-key-stdin is not supported in rpc mode"}
	}
	if common.timeout <= 0 {
		return &igwerr.UsageError{Msg: "--timeout must be positive"}
	}
	if workers <= 0 {
		return &igwerr.UsageError{Msg: "--workers must be >= 1"}
	}
	if queueSize <= 0 {
		return &igwerr.UsageError{Msg: "--queue-size must be >= 1"}
	}

	runner := rpcSessionRunner{
		cli:       c,
		common:    common,
		specFile:  specFile,
		workers:   workers,
		queueSize: queueSize,
	}
	return runner.run()
}

func withRPCCallQueueStats(resp rpcResponse, queueWaitMs int64, queueDepth int) rpcResponse {
	data, ok := resp.Data.(map[string]any)
	if !ok || data == nil {
		return resp
	}

	stats, ok := data["stats"].(callStats)
	if !ok {
		return resp
	}

	stats = withRPCQueueStats(stats, queueWaitMs, queueDepth)
	data["stats"] = stats
	resp.Data = data
	return resp
}

type rpcCapabilityArgs struct {
	Name string `json:"name,omitempty"`
}

type rpcCancelArgs struct {
	ID        any `json:"id,omitempty"`
	RequestID any `json:"requestId,omitempty"`
}

func (c *CLI) handleRPCCapability(req rpcRequest) rpcResponse {
	features := rpcFeatureFlags()
	if len(req.Args) == 0 {
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{
				"protocol":       rpcProtocolName,
				"protocolSemver": rpcProtocolSemver,
				"features":       features,
			},
		}
	}

	var args rpcCapabilityArgs
	if err := json.Unmarshal(req.Args, &args); err != nil {
		usageErr := &igwerr.UsageError{Msg: fmt.Sprintf("invalid capability args: %v", err)}
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(usageErr),
			Error: usageErr.Error(),
		}
	}

	normalized := strings.TrimSpace(args.Name)
	supported := false
	if normalized != "" {
		if direct, ok := features[normalized]; ok {
			supported = direct
		} else {
			for name, present := range features {
				if strings.EqualFold(name, normalized) {
					supported = present
					normalized = name
					break
				}
			}
		}
	}

	return rpcResponse{
		ID:   req.ID,
		OK:   true,
		Code: 0,
		Data: map[string]any{
			"name":      normalized,
			"supported": supported,
		},
	}
}

func (c *CLI) handleRPCCancel(req rpcRequest, session *rpcSessionState) rpcResponse {
	var args rpcCancelArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			usageErr := &igwerr.UsageError{Msg: fmt.Sprintf("invalid cancel args: %v", err)}
			return rpcResponse{
				ID:    req.ID,
				OK:    false,
				Code:  igwerr.ExitCode(usageErr),
				Error: usageErr.Error(),
			}
		}
	}

	target := args.RequestID
	if target == nil {
		target = args.ID
	}
	targetKey, ok := rpcRequestIDKey(target)
	if !ok {
		usageErr := &igwerr.UsageError{Msg: "cancel args require id or requestId"}
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(usageErr),
			Error: usageErr.Error(),
		}
	}

	return rpcResponse{
		ID:   req.ID,
		OK:   true,
		Code: 0,
		Data: map[string]any{
			"id":        targetKey,
			"cancelled": session.cancelRequest(targetKey),
		},
	}
}

func (c *CLI) handleRPCCall(req rpcRequest, common wrapperCommon, specFile string, session *rpcSessionState) rpcResponse {
	var item callBatchItem
	if len(req.Args) == 0 {
		item = callBatchItem{}
	} else if err := json.Unmarshal(req.Args, &item); err != nil {
		usageErr := &igwerr.UsageError{Msg: fmt.Sprintf("invalid call args: %v", err)}
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(usageErr),
			Error: usageErr.Error(),
		}
	}

	resolved, err := c.resolveRuntimeConfig(common.profile, common.gatewayURL, common.apiKey)
	if err != nil {
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(err),
			Error: err.Error(),
		}
	}

	defaults := callBatchDefaults{
		SpecFile:   specFile,
		Profile:    common.profile,
		GatewayURL: common.gatewayURL,
		APIKey:     common.apiKey,
	}

	opMap, opErr := c.loadBatchOperationMap([]callBatchItem{item}, defaults)
	if opErr != nil {
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(opErr),
			Error: opErr.Error(),
		}
	}

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.runtimeHTTPClient(),
	}

	input, parseErr := buildCallExecutionInputFromItem(item, callItemExecutionDefaults{
		Timeout:      common.timeout,
		Retry:        0,
		RetryBackoff: 250 * time.Millisecond,
		Yes:          false,
		OperationMap: opMap,
		EnableTiming: true,
	})
	if parseErr != nil {
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(parseErr),
			Error: parseErr.Error(),
		}
	}

	callCtx, callCancel := context.WithCancel(context.Background())
	defer callCancel()
	if reqKey, ok := session.registerInFlight(req.ID, callCancel); ok {
		defer session.unregisterInFlight(reqKey)
	}

	start := time.Now()
	input.Context = callCtx
	callResp, method, path, callErr := executeCallCore(client, input)
	elapsedMs := time.Since(start).Milliseconds()
	if callErr != nil {
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(callErr),
			Error: callErr.Error(),
			Data: map[string]any{
				"request": callJSONRequest{
					Method: method,
					URL:    path,
				},
				"cancelled": errors.Is(callErr, context.Canceled),
				"stats":     buildCallStats(callResp, elapsedMs),
			},
		}
	}

	return rpcResponse{
		ID:     req.ID,
		OK:     true,
		Code:   0,
		Status: callResp.StatusCode,
		Data: map[string]any{
			"request": callJSONRequest{
				Method: callResp.Method,
				URL:    callResp.URL,
			},
			"response": callJSONResponse{
				Status:    callResp.StatusCode,
				Headers:   maybeHeaders(callResp.Headers, common.includeHeaders),
				Body:      string(callResp.Body),
				Bytes:     callResp.BodyBytes,
				Truncated: callResp.Truncated,
			},
			"timingMs": elapsedMs, // backward-compatible shorthand
			"stats":    buildCallStats(callResp, elapsedMs),
		},
	}
}
