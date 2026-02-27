package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	callResp := responseByID(t, responses, "c1")
	if callResp["ok"] != true {
		t.Fatalf("expected call ok response: %#v", callResp)
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
