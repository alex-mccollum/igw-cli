package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/buildinfo"
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

const (
	rpcProtocolName    = "igw-rpc-v1"
	rpcProtocolSemver  = "1.0.0"
	rpcProtocolMinHost = "1.0.0"
)

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

	scanner := bufio.NewScanner(c.In)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	type rpcWorkItem struct {
		req rpcRequest
	}

	workQueue := make(chan rpcWorkItem, queueSize)
	results := make(chan rpcResponse, queueSize)

	var workerWG sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for work := range workQueue {
				results <- c.handleRPCRequest(work.req, common, specFile)
			}
		}()
	}

	writeErrCh := make(chan error, 1)
	go func() {
		enc := json.NewEncoder(c.Out)
		enc.SetEscapeHTML(false)
		var writeErr error
		for result := range results {
			if writeErr == nil {
				if err := enc.Encode(result); err != nil {
					writeErr = igwerr.NewTransportError(err)
				}
			}
		}
		writeErrCh <- writeErr
	}()

	scanErr := error(nil)
	stopRead := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			results <- rpcResponse{
				OK:    false,
				Code:  igwerr.ExitCode(&igwerr.UsageError{Msg: "invalid rpc request json"}),
				Error: fmt.Sprintf("invalid rpc request json: %v", err),
			}
			continue
		}

		workQueue <- rpcWorkItem{req: req}
		if strings.EqualFold(strings.TrimSpace(req.Op), "shutdown") {
			stopRead = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		scanErr = igwerr.NewTransportError(err)
	}

	close(workQueue)
	workerWG.Wait()
	close(results)

	writeErr := <-writeErrCh
	if writeErr != nil {
		return writeErr
	}
	if scanErr != nil {
		return scanErr
	}
	if stopRead {
		return nil
	}
	return nil
}

func rpcFeatureFlags() map[string]bool {
	return map[string]bool{
		"hello":            true,
		"call":             true,
		"reloadConfig":     true,
		"capability":       true,
		"shutdown":         true,
		"rpcWorkers":       true,
		"rpcQueueSize":     true,
		"sharedCallCoreV1": true,
		"callStatsV1":      true,
	}
}

func (c *CLI) handleRPCRequest(req rpcRequest, common wrapperCommon, specFile string) rpcResponse {
	switch strings.ToLower(strings.TrimSpace(req.Op)) {
	case "hello":
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{
				"protocol":       rpcProtocolName,
				"protocolSemver": rpcProtocolSemver,
				"minHostSemver":  rpcProtocolMinHost,
				"version":        buildinfo.Long(),
				"features":       rpcFeatureFlags(),
				"ops":            []string{"hello", "capability", "call", "reload_config", "shutdown"},
			},
		}
	case "capability":
		return c.handleRPCCapability(req)
	case "reload_config":
		c.invalidateRuntimeCaches()
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{"reloaded": true},
		}
	case "shutdown":
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{"shutdown": true},
		}
	case "call":
		return c.handleRPCCall(req, common, specFile)
	default:
		err := &igwerr.UsageError{Msg: fmt.Sprintf("unknown rpc op %q", strings.TrimSpace(req.Op))}
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(err),
			Error: err.Error(),
		}
	}
}

type rpcCapabilityArgs struct {
	Name string `json:"name,omitempty"`
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

func (c *CLI) handleRPCCall(req rpcRequest, common wrapperCommon, specFile string) rpcResponse {
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

	timeout := common.timeout
	if raw := strings.TrimSpace(item.Timeout); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			usageErr := &igwerr.UsageError{Msg: fmt.Sprintf("invalid timeout %q", raw)}
			return rpcResponse{
				ID:    req.ID,
				OK:    false,
				Code:  igwerr.ExitCode(usageErr),
				Error: usageErr.Error(),
			}
		}
		timeout = parsed
	}

	retry := 0
	if item.Retry != nil {
		retry = *item.Retry
	}
	retryBackoff := 250 * time.Millisecond
	if raw := strings.TrimSpace(item.RetryBackoff); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			usageErr := &igwerr.UsageError{Msg: fmt.Sprintf("invalid retryBackoff %q", raw)}
			return rpcResponse{
				ID:    req.ID,
				OK:    false,
				Code:  igwerr.ExitCode(usageErr),
				Error: usageErr.Error(),
			}
		}
		retryBackoff = parsed
	}

	yes := false
	if item.Yes != nil {
		yes = *item.Yes
	}

	start := time.Now()
	callResp, method, path, callErr := executeCallCore(client, callExecutionInput{
		Method:       item.Method,
		Path:         item.Path,
		OperationID:  item.OperationID,
		OperationMap: opMap,
		Query:        item.Query,
		Headers:      item.Headers,
		Body:         []byte(item.Body),
		ContentType:  item.ContentType,
		DryRun:       item.DryRun,
		Yes:          yes,
		Timeout:      timeout,
		Retry:        retry,
		RetryBackoff: retryBackoff,
		EnableTiming: true,
	})
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
				"stats": buildCallStats(callResp, elapsedMs),
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
