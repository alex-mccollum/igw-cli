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

func TestGatewayInfoWrapper(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    new(bytes.Buffer),
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	if err := c.Execute([]string{
		"gateway", "info",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
	}); err != nil {
		t.Fatalf("gateway info failed: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/data/api/v1/gateway-info" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
}

func TestScanProjectsWrapperRequiresYes(t *testing.T) {
	t.Parallel()

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    new(bytes.Buffer),
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
	}

	err := c.Execute([]string{
		"scan", "projects",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
	})
	if err == nil {
		t.Fatalf("expected --yes requirement failure")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestGatewayInfoWrapperPropagatesFieldsAndCompact(t *testing.T) {
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

	if err := c.Execute([]string{
		"gateway", "info",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--compact",
		"--select", "response.status",
	}); err != nil {
		t.Fatalf("gateway info failed: %v", err)
	}

	output := out.String()
	if !json.Valid([]byte(output)) {
		t.Fatalf("expected valid json output, got %q", output)
	}
	if strings.Contains(output, "\n  ") {
		t.Fatalf("expected compact output, got %q", output)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if int(payload["response.status"].(float64)) != 200 {
		t.Fatalf("unexpected response.status %#v", payload["response.status"])
	}
}

func TestGatewayInfoWrapperPropagatesRawSelect(t *testing.T) {
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

	if err := c.Execute([]string{
		"gateway", "info",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "response.status",
		"--raw",
	}); err != nil {
		t.Fatalf("gateway info failed: %v", err)
	}

	if out.String() != "200\n" {
		t.Fatalf("unexpected raw output %q", out.String())
	}
}
