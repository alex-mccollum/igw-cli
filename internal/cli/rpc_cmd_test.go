package cli

import (
	"bytes"
	"encoding/json"
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

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 rpc responses, got %d: %q", len(lines), out.String())
	}

	var hello map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &hello); err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if hello["ok"] != true {
		t.Fatalf("expected hello ok response: %#v", hello)
	}

	var callResp map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &callResp); err != nil {
		t.Fatalf("decode call: %v", err)
	}
	if callResp["ok"] != true {
		t.Fatalf("expected call ok response: %#v", callResp)
	}
}
