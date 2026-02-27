package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runWait(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw wait <gateway|diagnostics-bundle|restart-tasks> [flags]")
		return &igwerr.UsageError{Msg: "required wait target"}
	}

	switch args[0] {
	case "gateway":
		return c.runWaitTarget("gateway", "healthy", args[1:])
	case "diagnostics-bundle":
		return c.runWaitTarget("diagnostics-bundle", "ready", args[1:])
	case "restart-tasks":
		return c.runWaitTarget("restart-tasks", "clear", args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown wait target %q", args[0])}
	}
}

func (c *CLI) runWaitTarget(target string, suffix string, args []string) error {
	jsonRequested := argsWantJSON(args)
	fs := flag.NewFlagSet("wait "+target, flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var interval time.Duration
	var waitTimeout time.Duration
	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, false)
	fs.DurationVar(&interval, "interval", 2*time.Second, "Polling interval")
	fs.DurationVar(&waitTimeout, "wait-timeout", 2*time.Minute, "Maximum total wait time")

	condition := ""
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		condition = strings.TrimSpace(args[0])
		args = args[1:]
	}

	if jsonRequested {
		fs.SetOutput(io.Discard)
	}
	if err := fs.Parse(args); err != nil {
		parseErr := &igwerr.UsageError{Msg: err.Error()}
		if jsonRequested {
			return c.printWaitError(true, jsonSelectOptions{}, parseErr)
		}
		return parseErr
	}

	selectOpts, selectErr := newJSONSelectOptions(common.jsonOutput, common.compactJSON, common.rawOutput, common.selectors)
	if selectErr != nil {
		return c.printWaitError(common.jsonOutput, selectionErrorOptions(selectOpts), selectErr)
	}

	if fs.NArg() > 0 {
		if condition == "" && fs.NArg() == 1 {
			condition = strings.TrimSpace(fs.Arg(0))
		} else {
			return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "unexpected positional arguments"})
		}
	}
	if condition != "" && condition != suffix {
		return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{
			Msg: fmt.Sprintf("unexpected wait condition %q for target %q (expected %q)", condition, target, suffix),
		})
	}

	if common.apiKeyStdin {
		if common.apiKey != "" {
			return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printWaitError(common.jsonOutput, selectOpts, igwerr.NewTransportError(err))
		}
		common.apiKey = strings.TrimSpace(string(tokenBytes))
	}

	if interval <= 0 {
		return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--interval must be positive"})
	}
	if waitTimeout <= 0 {
		return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--wait-timeout must be positive"})
	}

	resolved, err := c.resolveRuntimeConfig(common.profile, common.gatewayURL, common.apiKey)
	if err != nil {
		return c.printWaitError(common.jsonOutput, selectOpts, err)
	}
	if strings.TrimSpace(resolved.GatewayURL) == "" {
		return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"})
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"})
	}
	if common.timeout <= 0 {
		return c.printWaitError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "--timeout must be positive"})
	}

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.runtimeHTTPClient(),
	}

	check := waitCheckForTarget(client, target, common.timeout)
	start := time.Now()
	result, waitErr := runWaitLoop(check, target, suffix, interval, waitTimeout)
	if waitErr != nil {
		return c.printWaitError(common.jsonOutput, selectOpts, waitErr)
	}
	result.ElapsedMs = time.Since(start).Milliseconds()
	stats := map[string]any{}
	if common.timing || common.jsonStats {
		stats["attempts"] = result.Attempts
		stats["elapsedMs"] = result.ElapsedMs
		if result.LastHTTP != nil {
			stats["lastHTTP"] = result.LastHTTP
		}
	}

	if common.jsonOutput {
		payload := map[string]any{
			"ok":        true,
			"target":    result.Target,
			"condition": result.Condition,
			"ready":     true,
			"attempts":  result.Attempts,
			"elapsedMs": result.ElapsedMs,
			"state":     result.State,
			"message":   result.Message,
		}
		if common.jsonStats || common.timing {
			payload["stats"] = stats
		}
		if selectWriteErr := printJSONSelection(c.Out, payload, selectOpts); selectWriteErr != nil {
			return c.printWaitError(common.jsonOutput, selectionErrorOptions(selectOpts), selectWriteErr)
		}
		return nil
	}

	fmt.Fprintf(c.Out, "ready\t%s\t%s\tattempts=%d\telapsed=%s\t%s\n",
		result.Target,
		result.Condition,
		result.Attempts,
		time.Duration(result.ElapsedMs)*time.Millisecond,
		result.Message,
	)
	if common.timing {
		fmt.Fprintf(c.Err, "timing\tattempts=%d\telapsedMs=%d\n", result.Attempts, result.ElapsedMs)
	}

	return nil
}

type waitObservation struct {
	Ready   bool
	Message string
	State   map[string]any
	HTTP    *gateway.CallTiming
}

type waitResult struct {
	Target    string
	Condition string
	Attempts  int
	ElapsedMs int64
	Message   string
	State     map[string]any
	LastHTTP  *gateway.CallTiming
}

type waitCheck func() (waitObservation, error)

type waitTerminalError struct {
	err error
}

func (e *waitTerminalError) Error() string {
	return e.err.Error()
}

func (e *waitTerminalError) Unwrap() error {
	return e.err
}

func newWaitTerminalError(err error) error {
	if err == nil {
		return nil
	}
	return &waitTerminalError{err: err}
}

func runWaitLoop(check waitCheck, target string, condition string, interval time.Duration, waitTimeout time.Duration) (waitResult, error) {
	deadline := time.Now().Add(waitTimeout)
	attempts := 0
	var lastObservation waitObservation
	var lastErr error
	sleep := interval
	maxSleep := adaptiveWaitMaxInterval(interval)

	for {
		attempts++

		observation, err := check()
		if err == nil {
			lastObservation = observation
			if observation.Ready {
				return waitResult{
					Target:    target,
					Condition: condition,
					Attempts:  attempts,
					Message:   observation.Message,
					State:     observation.State,
					LastHTTP:  observation.HTTP,
				}, nil
			}
		} else {
			lastErr = err

			var terminalErr *waitTerminalError
			if errors.As(err, &terminalErr) {
				return waitResult{}, terminalErr.err
			}
			if !retryableWaitError(err) {
				return waitResult{}, err
			}
		}

		if time.Now().After(deadline) {
			msg := fmt.Sprintf("timed out waiting for %s to become %s", target, condition)
			if lastErr != nil {
				msg += fmt.Sprintf(" (last error: %v)", lastErr)
			} else if lastObservation.Message != "" {
				msg += fmt.Sprintf(" (last state: %s)", lastObservation.Message)
			}
			return waitResult{}, igwerr.NewTransportError(errors.New(msg))
		}

		time.Sleep(sleep)
		sleep = nextAdaptiveWaitInterval(sleep, maxSleep)
	}
}

func adaptiveWaitMaxInterval(base time.Duration) time.Duration {
	max := base * 4
	if max < 2*time.Second {
		max = 2 * time.Second
	}
	if max > 30*time.Second {
		max = 30 * time.Second
	}
	return max
}

func nextAdaptiveWaitInterval(current time.Duration, max time.Duration) time.Duration {
	next := current + current/2
	if next > max {
		return max
	}
	return next
}

func retryableWaitError(err error) bool {
	var usageErr *igwerr.UsageError
	if errors.As(err, &usageErr) {
		return false
	}

	var statusErr *igwerr.StatusError
	if errors.As(err, &statusErr) {
		return !statusErr.AuthFailure()
	}

	return true
}

func waitCheckForTarget(client *gateway.Client, target string, timeout time.Duration) waitCheck {
	switch target {
	case "gateway":
		return func() (waitObservation, error) {
			resp, err := client.Call(context.Background(), gateway.CallRequest{
				Method:       "GET",
				Path:         "/data/api/v1/gateway-info",
				Timeout:      timeout,
				EnableTiming: true,
			})
			if err != nil {
				return waitObservation{}, err
			}
			return waitObservation{
				Ready:   true,
				Message: "gateway responded with HTTP 200",
				State: map[string]any{
					"status": resp.StatusCode,
				},
				HTTP: resp.Timing,
			}, nil
		}
	case "diagnostics-bundle":
		return func() (waitObservation, error) {
			resp, err := client.Call(context.Background(), gateway.CallRequest{
				Method:       "GET",
				Path:         "/data/api/v1/diagnostics/bundle/status",
				Timeout:      timeout,
				EnableTiming: true,
			})
			if err != nil {
				return waitObservation{}, err
			}
			body, err := decodeDiagnosticsStatusBody(resp.Body)
			if err != nil {
				return waitObservation{}, err
			}

			state := strings.ToUpper(strings.TrimSpace(body.State))
			fileSize := body.FileSize
			ready := fileSize > 0 || diagnosticsReadyState(state)
			message := fmt.Sprintf("state=%s fileSize=%d", state, fileSize)

			if diagnosticsFailedState(state) {
				return waitObservation{}, newWaitTerminalError(
					igwerr.NewTransportError(fmt.Errorf("diagnostics bundle failed with state %q", state)),
				)
			}

			return waitObservation{
				Ready:   ready,
				Message: message,
				State: map[string]any{
					"state":    state,
					"fileSize": fileSize,
				},
				HTTP: resp.Timing,
			}, nil
		}
	default: // restart-tasks
		return func() (waitObservation, error) {
			resp, err := client.Call(context.Background(), gateway.CallRequest{
				Method:       "GET",
				Path:         "/data/api/v1/restart-tasks/pending",
				Timeout:      timeout,
				EnableTiming: true,
			})
			if err != nil {
				return waitObservation{}, err
			}
			body, err := decodePendingTasksBody(resp.Body)
			if err != nil {
				return waitObservation{}, err
			}

			pending := body.Pending
			ready := len(pending) == 0

			return waitObservation{
				Ready:   ready,
				Message: fmt.Sprintf("pending=%d", len(pending)),
				State: map[string]any{
					"pending": pending,
				},
				HTTP: resp.Timing,
			}, nil
		}
	}
}

func diagnosticsReadyState(state string) bool {
	switch state {
	case "COMPLETE", "COMPLETED", "READY", "DONE":
		return true
	default:
		return false
	}
}

func diagnosticsFailedState(state string) bool {
	switch state {
	case "ERROR", "FAILED", "FAILURE":
		return true
	default:
		return false
	}
}

func decodeJSONBody(body []byte) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, igwerr.NewTransportError(fmt.Errorf("decode response json: %w", err))
	}
	return out, nil
}

type diagnosticsStatusBody struct {
	State    string `json:"state"`
	FileSize int    `json:"fileSize"`
}

func decodeDiagnosticsStatusBody(body []byte) (diagnosticsStatusBody, error) {
	var out diagnosticsStatusBody
	if err := json.Unmarshal(body, &out); err != nil {
		return diagnosticsStatusBody{}, igwerr.NewTransportError(fmt.Errorf("decode response json: %w", err))
	}
	return out, nil
}

type pendingTasksBody struct {
	Pending []string `json:"pending"`
}

func decodePendingTasksBody(body []byte) (pendingTasksBody, error) {
	var out pendingTasksBody
	if err := json.Unmarshal(body, &out); err != nil {
		return pendingTasksBody{}, igwerr.NewTransportError(fmt.Errorf("decode response json: %w", err))
	}
	return out, nil
}

func intFromMap(m map[string]any, key string) int {
	value, ok := m[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func stringFromMap(m map[string]any, key string) string {
	value, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func stringSliceFromMap(m map[string]any, key string) []string {
	value, ok := m[key]
	if !ok {
		return nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (c *CLI) printWaitError(jsonOutput bool, selectOpts jsonSelectOptions, err error) error {
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
