package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestRPCModeHelloCallShutdown(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In: strings.NewReader(strings.Join([]string{
			`{"id":"h1","op":"hello"}`,
			`{"id":"c1","op":"call","args":{"method":"GET","path":"/data/api/v1/gateway-info"}}`,
			`{"id":"s1","op":"shutdown"}`,
		}, "\n")),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	if err := c.Execute([]string{"rpc", "--gateway-url", srv.URL, "--api-key", "secret"}); err != nil {
		t.Fatalf("rpc failed: %v", err)
	}

	responses := decodeRPCResponses(t, out.String())
	if len(responses) != 3 {
		t.Fatalf("expected 3 rpc responses, got %d: %q", len(responses), out.String())
	}

	hello := responseByID(t, responses, "h1")
	if hello["ok"] != true {
		t.Fatalf("expected hello ok response: %#v", hello)
	}
	helloData, ok := hello["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected hello response data payload: %#v", hello)
	}
	if helloData["protocol"] != rpcProtocolName {
		t.Fatalf("expected protocol %q in hello response: %#v", rpcProtocolName, helloData)
	}
	if helloData["protocolSemver"] != rpcProtocolSemver {
		t.Fatalf("expected protocolSemver %q in hello response: %#v", rpcProtocolSemver, helloData)
	}
	features, ok := helloData["features"].(map[string]any)
	if !ok {
		t.Fatalf("expected hello features payload: %#v", helloData)
	}
	if features["capability"] != true {
		t.Fatalf("expected capability feature in hello response: %#v", features)
	}
	if features["cancel"] != true {
		t.Fatalf("expected cancel feature in hello response: %#v", features)
	}

	callResp := responseByID(t, responses, "c1")
	if callResp["ok"] != true {
		t.Fatalf("expected call ok response: %#v", callResp)
	}
	callData, ok := callResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected call response data payload: %#v", callResp)
	}
	callStats, ok := callData["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected call stats payload: %#v", callData)
	}
	rpcStats, ok := callStats["rpc"].(map[string]any)
	if !ok {
		t.Fatalf("expected rpc queue stats in call payload: %#v", callStats)
	}
	if queueWait, ok := rpcStats["queueWaitMs"].(float64); !ok || queueWait < 0 {
		t.Fatalf("expected non-negative queueWaitMs in rpc stats: %#v", rpcStats)
	}
	if queueDepth, ok := rpcStats["queueDepth"].(float64); !ok || queueDepth < 0 {
		t.Fatalf("expected non-negative queueDepth in rpc stats: %#v", rpcStats)
	}
}

func TestRPCModeCapabilityOperation(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		In: strings.NewReader(strings.Join([]string{
			`{"id":"c1","op":"capability","args":{"name":"rpcWorkers"}}`,
			`{"id":"c2","op":"capability","args":{"name":"nope"}}`,
			`{"id":"s1","op":"shutdown"}`,
		}, "\n")),
		Out: &out,
		Err: new(bytes.Buffer),
		Getenv: func(string) string {
			return ""
		},
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
	}

	if err := c.Execute([]string{"rpc", "--gateway-url", "http://127.0.0.1:8088", "--api-key", "secret"}); err != nil {
		t.Fatalf("rpc failed: %v", err)
	}

	responses := decodeRPCResponses(t, out.String())
	if len(responses) != 3 {
		t.Fatalf("expected 3 rpc responses, got %d: %q", len(responses), out.String())
	}

	supported := responseByID(t, responses, "c1")
	supportedData, ok := supported["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected capability data payload: %#v", supported)
	}
	if supportedData["name"] != "rpcWorkers" || supportedData["supported"] != true {
		t.Fatalf("expected supported capability response: %#v", supportedData)
	}

	unsupported := responseByID(t, responses, "c2")
	unsupportedData, ok := unsupported["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected capability data payload: %#v", unsupported)
	}
	if unsupportedData["name"] != "nope" || unsupportedData["supported"] != false {
		t.Fatalf("expected unsupported capability response: %#v", unsupportedData)
	}
}

func TestRPCModeValidatesQueueAndWorkers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		msg  string
	}{
		{
			name: "workers",
			args: []string{"rpc", "--gateway-url", "http://127.0.0.1:8088", "--api-key", "secret", "--workers", "0"},
			msg:  "--workers must be >= 1",
		},
		{
			name: "queue-size",
			args: []string{"rpc", "--gateway-url", "http://127.0.0.1:8088", "--api-key", "secret", "--queue-size", "0"},
			msg:  "--queue-size must be >= 1",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &CLI{
				In:  strings.NewReader(""),
				Out: new(bytes.Buffer),
				Err: new(bytes.Buffer),
				Getenv: func(string) string {
					return ""
				},
				ReadConfig: func() (config.File, error) {
					return config.File{}, nil
				},
			}

			err := c.Execute(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected error containing %q, got %v", tc.msg, err)
			}
		})
	}
}

func TestRPCModeRecoversFromMalformedRequest(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		In: strings.NewReader(strings.Join([]string{
			`{"op":"hello","id":"ok-1"}`,
			`{"op":"call","id":"broken",`,
			`{"op":"shutdown","id":"s1"}`,
		}, "\n")),
		Out: &out,
		Err: new(bytes.Buffer),
		Getenv: func(string) string {
			return ""
		},
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
	}

	if err := c.Execute([]string{"rpc", "--gateway-url", "http://127.0.0.1:8088", "--api-key", "secret"}); err != nil {
		t.Fatalf("rpc failed: %v", err)
	}

	responses := decodeRPCResponses(t, out.String())
	if len(responses) != 3 {
		t.Fatalf("expected 3 rpc responses, got %d: %q", len(responses), out.String())
	}

	hello := responseByID(t, responses, "ok-1")
	if hello["ok"] != true {
		t.Fatalf("expected hello ok response: %#v", hello)
	}
	shutdown := responseByID(t, responses, "s1")
	if shutdown["ok"] != true {
		t.Fatalf("expected shutdown ok response: %#v", shutdown)
	}

	var malformed map[string]any
	for _, resp := range responses {
		if _, ok := resp["id"]; ok {
			continue
		}
		malformed = resp
		break
	}
	if malformed == nil {
		t.Fatalf("expected malformed request response in output: %#v", responses)
	}
	if malformed["ok"] != false {
		t.Fatalf("expected malformed request to be marked not ok: %#v", malformed)
	}
	if code, _ := malformed["code"].(float64); int(code) != 2 {
		t.Fatalf("expected usage error code 2 for malformed request: %#v", malformed)
	}
	if !strings.Contains(fmt.Sprint(malformed["error"]), "invalid rpc request json") {
		t.Fatalf("expected malformed request error message: %#v", malformed)
	}
}

func TestRPCModeStopsReadingAfterShutdown(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		In: strings.NewReader(strings.Join([]string{
			`{"op":"hello","id":"h1"}`,
			`{"op":"shutdown","id":"s1"}`,
			`{"op":"hello","id":"h2"}`,
		}, "\n")),
		Out: &out,
		Err: new(bytes.Buffer),
		Getenv: func(string) string {
			return ""
		},
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
	}

	if err := c.Execute([]string{
		"rpc", "--gateway-url", "http://127.0.0.1:8088", "--api-key", "secret", "--workers", "2", "--queue-size", "1",
	}); err != nil {
		t.Fatalf("rpc failed: %v", err)
	}

	responses := decodeRPCResponses(t, out.String())
	if len(responses) != 2 {
		t.Fatalf("expected 2 rpc responses, got %d: %q", len(responses), out.String())
	}
	if responseExistsByID(responses, "h2") {
		t.Fatalf("expected input after shutdown to be ignored: %#v", responses)
	}
	if !responseExistsByID(responses, "h1") {
		t.Fatalf("expected hello response before shutdown: %#v", responses)
	}
	if !responseExistsByID(responses, "s1") {
		t.Fatalf("expected shutdown response: %#v", responses)
	}
}

func TestRPCModeCancelsInFlightCall(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	inReader, inWriter := io.Pipe()
	var out bytes.Buffer
	c := &CLI{
		In:  inReader,
		Out: &out,
		Err: new(bytes.Buffer),
		Getenv: func(string) string {
			return ""
		},
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- c.Execute([]string{
			"rpc", "--gateway-url", srv.URL, "--api-key", "secret", "--workers", "2", "--queue-size", "4",
		})
	}()

	_, _ = io.WriteString(inWriter, `{"id":"call-1","op":"call","args":{"method":"GET","path":"/data/api/v1/gateway-info","timeout":"5s"}}`+"\n")
	time.Sleep(50 * time.Millisecond)
	_, _ = io.WriteString(inWriter, `{"id":"cancel-1","op":"cancel","args":{"id":"call-1"}}`+"\n")
	_, _ = io.WriteString(inWriter, `{"id":"s1","op":"shutdown"}`+"\n")
	_ = inWriter.Close()

	if err := <-runErr; err != nil {
		t.Fatalf("rpc failed: %v", err)
	}

	responses := decodeRPCResponses(t, out.String())
	if len(responses) != 3 {
		t.Fatalf("expected 3 rpc responses, got %d: %q", len(responses), out.String())
	}

	cancelResp := responseByID(t, responses, "cancel-1")
	cancelData, ok := cancelResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected cancel response data payload: %#v", cancelResp)
	}
	if cancelData["cancelled"] != true {
		t.Fatalf("expected cancel op to cancel in-flight request: %#v", cancelData)
	}

	callResp := responseByID(t, responses, "call-1")
	if callResp["ok"] != false {
		t.Fatalf("expected cancelled call to fail: %#v", callResp)
	}
	if !strings.Contains(fmt.Sprint(callResp["error"]), "context canceled") {
		t.Fatalf("expected context cancellation error for cancelled call: %#v", callResp)
	}
	callData, ok := callResp["data"].(map[string]any)
	if !ok || callData["cancelled"] != true {
		t.Fatalf("expected cancelled marker in call response data: %#v", callResp)
	}
	stats, ok := callData["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected stats payload on cancelled call response: %#v", callData)
	}
	rpcStats, ok := stats["rpc"].(map[string]any)
	if !ok {
		t.Fatalf("expected rpc queue stats on cancelled call response: %#v", stats)
	}
	if queueWait, ok := rpcStats["queueWaitMs"].(float64); !ok || queueWait < 0 {
		t.Fatalf("expected non-negative queueWaitMs on cancelled call response: %#v", rpcStats)
	}
	if queueDepth, ok := rpcStats["queueDepth"].(float64); !ok || queueDepth < 0 {
		t.Fatalf("expected non-negative queueDepth on cancelled call response: %#v", rpcStats)
	}
}

func decodeRPCResponses(t *testing.T, out string) []map[string]any {
	t.Helper()

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("decode rpc response %q: %v", line, err)
		}
		responses = append(responses, payload)
	}
	return responses
}

func responseByID(t *testing.T, responses []map[string]any, id string) map[string]any {
	t.Helper()

	for _, resp := range responses {
		if fmt.Sprint(resp["id"]) == id {
			return resp
		}
	}
	t.Fatalf("response id %q not found in %#v", id, responses)
	return nil
}

func responseExistsByID(responses []map[string]any, id string) bool {
	for _, resp := range responses {
		if fmt.Sprint(resp["id"]) == id {
			return true
		}
	}
	return false
}
