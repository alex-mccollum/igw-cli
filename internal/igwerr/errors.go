package igwerr

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/alex-mccollum/igw-cli/internal/exitcode"
)

type UsageError struct {
	Msg string
}

func (e *UsageError) Error() string {
	return e.Msg
}

type StatusError struct {
	StatusCode int
	Body       string
	Hint       string
}

func (e *StatusError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("http %d: %s", e.StatusCode, e.Hint)
	}

	return fmt.Sprintf("http %d", e.StatusCode)
}

func (e *StatusError) AuthFailure() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

type TransportError struct {
	Err     error
	Timeout bool
}

func (e *TransportError) Error() string {
	if e.Timeout {
		return "network timeout"
	}

	if e.Err != nil {
		return fmt.Sprintf("network error: %v", e.Err)
	}

	return "network error"
}

func (e *TransportError) Unwrap() error {
	return e.Err
}

func NewTransportError(err error) *TransportError {
	var netErr net.Error
	return &TransportError{
		Err:     err,
		Timeout: errors.As(err, &netErr) && netErr.Timeout(),
	}
}

func ExitCode(err error) int {
	if err == nil {
		return exitcode.Success
	}

	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return exitcode.Usage
	}

	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		if statusErr.AuthFailure() {
			return exitcode.Auth
		}

		return exitcode.Network
	}

	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		return exitcode.Network
	}

	return exitcode.Network
}
