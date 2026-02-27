package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
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

func (c *CLI) runRPC(args []string) error {
	fs := flag.NewFlagSet("rpc", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var specFile string
	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, false)
	fs.StringVar(&specFile, "spec-file", "openapi.json", "Path to OpenAPI JSON file (for op-based call resolution)")

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

	scanner := bufio.NewScanner(c.In)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(c.Out)
	enc.SetEscapeHTML(false)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(rpcResponse{
				OK:    false,
				Code:  igwerr.ExitCode(&igwerr.UsageError{Msg: "invalid rpc request json"}),
				Error: fmt.Sprintf("invalid rpc request json: %v", err),
			})
			continue
		}

		resp, shutdown := c.handleRPCRequest(req, common, specFile)
		if err := enc.Encode(resp); err != nil {
			return igwerr.NewTransportError(err)
		}
		if shutdown {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return igwerr.NewTransportError(err)
	}
	return nil
}

func (c *CLI) handleRPCRequest(req rpcRequest, common wrapperCommon, specFile string) (rpcResponse, bool) {
	switch strings.ToLower(strings.TrimSpace(req.Op)) {
	case "hello":
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{
				"protocol": "igw-rpc-v1",
				"version":  buildinfo.Long(),
				"ops":      []string{"hello", "call", "reload_config", "shutdown"},
			},
		}, false
	case "reload_config":
		c.invalidateRuntimeCaches()
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{"reloaded": true},
		}, false
	case "shutdown":
		return rpcResponse{
			ID:   req.ID,
			OK:   true,
			Code: 0,
			Data: map[string]any{"shutdown": true},
		}, true
	case "call":
		return c.handleRPCCall(req, common, specFile), false
	default:
		err := &igwerr.UsageError{Msg: fmt.Sprintf("unknown rpc op %q", strings.TrimSpace(req.Op))}
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(err),
			Error: err.Error(),
		}, false
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
		Retry:        0,
		RetryBackoff: 250 * time.Millisecond,
		Timeout:      common.timeout,
		Yes:          false,
		SpecFile:     specFile,
		Profile:      common.profile,
		GatewayURL:   common.gatewayURL,
		APIKey:       common.apiKey,
		IncludeHeads: common.includeHeaders,
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
	result := c.executeBatchCallItem(client, 0, item, defaults, opMap)
	resp := rpcResponse{
		ID:     req.ID,
		OK:     result.OK,
		Code:   result.Code,
		Status: result.Status,
	}
	if result.OK {
		resp.Data = map[string]any{
			"request":  result.Request,
			"response": result.Response,
			"timingMs": result.TimingMs,
		}
		return resp
	}
	resp.Error = result.Error
	return resp
}

func writeRPCResponse(w io.Writer, resp rpcResponse) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}
