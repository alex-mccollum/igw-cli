package cli

import (
	"fmt"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/buildinfo"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const (
	rpcProtocolName    = "igw-rpc-v1"
	rpcProtocolSemver  = "1.0.0"
	rpcProtocolMinHost = "1.0.0"
)

type rpcOperationHandler func(*CLI, rpcRequest, wrapperCommon, string, *rpcSessionState) rpcResponse

type rpcOperationDef struct {
	Name    string
	Feature string
	Handler rpcOperationHandler
}

func rpcOperationDefinitions() []rpcOperationDef {
	return []rpcOperationDef{
		{
			Name:    "hello",
			Feature: "hello",
			Handler: func(c *CLI, req rpcRequest, _ wrapperCommon, _ string, _ *rpcSessionState) rpcResponse {
				return c.handleRPCHello(req)
			},
		},
		{
			Name:    "capability",
			Feature: "capability",
			Handler: func(c *CLI, req rpcRequest, _ wrapperCommon, _ string, _ *rpcSessionState) rpcResponse {
				return c.handleRPCCapability(req)
			},
		},
		{
			Name:    "call",
			Feature: "call",
			Handler: func(c *CLI, req rpcRequest, common wrapperCommon, specFile string, session *rpcSessionState) rpcResponse {
				return c.handleRPCCall(req, common, specFile, session)
			},
		},
		{
			Name:    "cancel",
			Feature: "cancel",
			Handler: func(c *CLI, req rpcRequest, _ wrapperCommon, _ string, session *rpcSessionState) rpcResponse {
				return c.handleRPCCancel(req, session)
			},
		},
		{
			Name:    "reload_config",
			Feature: "reloadConfig",
			Handler: func(c *CLI, req rpcRequest, _ wrapperCommon, _ string, _ *rpcSessionState) rpcResponse {
				c.invalidateRuntimeCaches()
				return rpcResponse{
					ID:   req.ID,
					OK:   true,
					Code: 0,
					Data: map[string]any{"reloaded": true},
				}
			},
		},
		{
			Name:    "shutdown",
			Feature: "shutdown",
			Handler: func(c *CLI, req rpcRequest, _ wrapperCommon, _ string, _ *rpcSessionState) rpcResponse {
				return rpcResponse{
					ID:   req.ID,
					OK:   true,
					Code: 0,
					Data: map[string]any{"shutdown": true},
				}
			},
		},
	}
}

func rpcOperationNames() []string {
	operations := rpcOperationDefinitions()
	out := make([]string, 0, len(operations))
	for _, op := range operations {
		out = append(out, op.Name)
	}
	return out
}

func rpcFeatureFlags() map[string]bool {
	features := map[string]bool{
		"rpcWorkers":       true,
		"rpcQueueSize":     true,
		"sharedCallCoreV1": true,
		"callStatsV1":      true,
	}
	for _, op := range rpcOperationDefinitions() {
		feature := strings.TrimSpace(op.Feature)
		if feature == "" {
			continue
		}
		features[feature] = true
	}
	return features
}

func findRPCOperation(op string) (rpcOperationDef, bool) {
	normalized := strings.TrimSpace(op)
	for _, def := range rpcOperationDefinitions() {
		if strings.EqualFold(def.Name, normalized) {
			return def, true
		}
	}
	return rpcOperationDef{}, false
}

func (c *CLI) handleRPCRequest(req rpcRequest, common wrapperCommon, specFile string, session *rpcSessionState) rpcResponse {
	op, ok := findRPCOperation(req.Op)
	if !ok {
		err := &igwerr.UsageError{Msg: fmt.Sprintf("unknown rpc op %q", strings.TrimSpace(req.Op))}
		return rpcResponse{
			ID:    req.ID,
			OK:    false,
			Code:  igwerr.ExitCode(err),
			Error: err.Error(),
		}
	}
	return op.Handler(c, req, common, specFile, session)
}

func (c *CLI) handleRPCHello(req rpcRequest) rpcResponse {
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
			"ops":            rpcOperationNames(),
		},
	}
}
