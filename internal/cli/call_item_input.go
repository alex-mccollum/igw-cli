package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type callItemExecutionDefaults struct {
	Timeout      time.Duration
	Retry        int
	RetryBackoff time.Duration
	Yes          bool
	OperationMap map[string]apidocs.Operation
	EnableTiming bool
}

func buildCallExecutionInputFromItem(item callBatchItem, defaults callItemExecutionDefaults) (callExecutionInput, error) {
	timeout := defaults.Timeout
	if raw := strings.TrimSpace(item.Timeout); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			return callExecutionInput{}, &igwerr.UsageError{Msg: fmt.Sprintf("invalid timeout %q", raw)}
		}
		timeout = parsed
	}

	retry := defaults.Retry
	if item.Retry != nil {
		retry = *item.Retry
	}

	retryBackoff := defaults.RetryBackoff
	if raw := strings.TrimSpace(item.RetryBackoff); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			return callExecutionInput{}, &igwerr.UsageError{Msg: fmt.Sprintf("invalid retryBackoff %q", raw)}
		}
		retryBackoff = parsed
	}

	yes := defaults.Yes
	if item.Yes != nil {
		yes = *item.Yes
	}

	return callExecutionInput{
		Method:       item.Method,
		Path:         item.Path,
		OperationID:  item.OperationID,
		OperationMap: defaults.OperationMap,
		Query:        item.Query,
		Headers:      item.Headers,
		Body:         []byte(item.Body),
		ContentType:  item.ContentType,
		DryRun:       item.DryRun,
		Yes:          yes,
		Timeout:      timeout,
		Retry:        retry,
		RetryBackoff: retryBackoff,
		EnableTiming: defaults.EnableTiming,
	}, nil
}
