// Package result defines machine contracts independently of command rendering.
package result

import (
	"context"
	"errors"
	"net/http"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const Version = "igw/v1"

type Problem struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Code    int    `json:"exitCode"`
	Details any    `json:"details,omitempty"`
}

func (p *Problem) Error() string { return p.Message }
func (p *Problem) ExitCode() int { return p.Code }

type Metadata struct {
	Target       *catalog.Target   `json:"target,omitempty"`
	Catalog      *catalog.Metadata `json:"catalog,omitempty"`
	Stale        bool              `json:"stale,omitempty"`
	HTTPStatus   int               `json:"httpStatus,omitempty"`
	Verification string            `json:"verification,omitempty"`
	Warnings     []string          `json:"warnings,omitempty"`
}

type Result struct {
	Version  string         `json:"version"`
	OK       bool           `json:"ok"`
	Outcome  string         `json:"outcome"`
	Data     any            `json:"data"`
	Artifact *artifact.Info `json:"artifact,omitempty"`
	Error    *Problem       `json:"error,omitempty"`
	Meta     Metadata       `json:"meta"`
}

func Success(data any) Result {
	return Result{Version: Version, OK: true, Outcome: "completed", Data: data}
}
func Failure(err error) Result {
	return Result{Version: Version, OK: false, Outcome: "failed", Error: FromError(err)}
}
func Usage(message string) *Problem { return &Problem{Kind: "usage", Message: message, Code: 2} }

// Error responses intentionally omit transport URLs, response bodies, and
// instance values, any of which may contain credentials or Gateway secrets.
func FromError(err error) *Problem {
	var problem *Problem
	if errors.As(err, &problem) {
		return problem
	}
	var usage *igwerr.UsageError
	if errors.As(err, &usage) {
		return Usage(usage.Error())
	}
	var status *igwerr.StatusError
	if errors.As(err, &status) {
		kind, message, code := "http", "Gateway rejected the request", 7
		switch status.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			kind, message, code = "auth", "Gateway authentication or permission check failed", 6
		case http.StatusConflict, http.StatusPreconditionFailed:
			kind, message = "conflict", "Gateway state changed or conflicts with the request; read the current state before retrying"
		}
		return &Problem{Kind: kind, Message: message, Code: code, Details: map[string]int{"httpStatus": status.StatusCode}}
	}
	if errors.Is(err, context.Canceled) {
		return &Problem{Kind: "canceled", Message: "operation canceled", Code: 7}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Problem{Kind: "timeout", Message: "operation deadline exceeded", Code: 7}
	}
	var transport *igwerr.TransportError
	if errors.As(err, &transport) {
		return &Problem{Kind: "transport", Message: "Gateway transfer failed", Code: 7}
	}
	return &Problem{Kind: "local", Message: "local operation failed", Code: 7}
}
