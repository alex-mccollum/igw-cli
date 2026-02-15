package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestCallJSONContractSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	err := c.Execute([]string{
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--json",
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}

	response, ok := payload["response"].(map[string]any)
	if !ok {
		t.Fatalf("missing response object")
	}
	if int(response["status"].(float64)) != 200 {
		t.Fatalf("unexpected status %v", response["status"])
	}
}

func TestCallJSONContractUsageError(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
	}

	err := c.Execute([]string{
		"call",
		"--gateway-url", "http://127.0.0.1:8088",
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--json",
	})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false")
	}
	if int(payload["code"].(float64)) != 2 {
		t.Fatalf("unexpected code in envelope %v", payload["code"])
	}
}

func TestCallJSONContractAuthErrorDetails(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`forbidden`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	err := c.Execute([]string{
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--json",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}
	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("unexpected exit code %d", code)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if int(payload["code"].(float64)) != 6 {
		t.Fatalf("unexpected code in envelope %v", payload["code"])
	}
	details, ok := payload["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details in envelope")
	}
	if int(details["status"].(float64)) != 403 {
		t.Fatalf("unexpected status detail %v", details["status"])
	}
}
