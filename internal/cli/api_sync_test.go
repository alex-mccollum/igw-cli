package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestAPISyncWritesSpecToConfigDir(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi" {
			http.NotFound(w, r)
			return
		}
		gotToken = r.Header.Get("X-Ignition-API-Token")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(apiSpecFixture))
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
		"api", "sync",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
	}); err != nil {
		t.Fatalf("api sync failed: %v", err)
	}

	if gotToken != "secret" {
		t.Fatalf("expected token header, got %q", gotToken)
	}

	var payload struct {
		OK       bool   `json:"ok"`
		SpecPath string `json:"specPath"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true in payload")
	}
	if payload.SpecPath == "" {
		t.Fatalf("expected spec path in payload")
	}
	if _, err := os.Stat(payload.SpecPath); err != nil {
		t.Fatalf("expected spec file at %q: %v", payload.SpecPath, err)
	}
}

func TestAPIRefreshUsesSyncBehavior(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(apiSpecFixture))
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
		"api", "refresh",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--fields", "ok,operationCount",
		"--compact",
	}); err != nil {
		t.Fatalf("api refresh failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if int(payload["operationCount"].(float64)) < 1 {
		t.Fatalf("expected operationCount > 0, got %#v", payload["operationCount"])
	}
}

func TestAPISyncFallsBackToOpenAPIJSONPath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi":
			http.NotFound(w, r)
		case "/openapi.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(apiSpecFixture))
		default:
			http.NotFound(w, r)
		}
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
		"api", "sync",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--field", "sourceURL",
	}); err != nil {
		t.Fatalf("api sync failed: %v", err)
	}

	if !strings.Contains(strings.TrimSpace(out.String()), "/openapi.json") {
		t.Fatalf("expected /openapi.json source url, got %q", out.String())
	}
}

func TestAPIListAutoSyncsMissingDefaultSpec(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	emptyWD := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(emptyWD); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(apiSpecFixture))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{
				GatewayURL: srv.URL,
				Token:      "secret",
			}, nil
		},
		HTTPClient: srv.Client(),
	}

	if err := c.Execute([]string{"api", "list"}); err != nil {
		t.Fatalf("api list failed: %v", err)
	}

	if !strings.Contains(out.String(), "gatewayInfo") {
		t.Fatalf("expected operation listing, got %q", out.String())
	}

	specPath := filepath.Join(configHome, "igw", "openapi.json")
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("expected synced spec at %q: %v", specPath, err)
	}
}

func TestCallOperationIDAutoSyncsMissingDefaultSpec(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	emptyWD := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(emptyWD); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	var sawOpenAPI bool
	var sawGatewayInfo bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi":
			sawOpenAPI = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(callOpSpecFixture))
		case "/data/api/v1/gateway-info":
			sawGatewayInfo = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{
				GatewayURL: srv.URL,
				Token:      "secret",
			}, nil
		},
		HTTPClient: srv.Client(),
	}

	if err := c.Execute([]string{
		"call",
		"--op", "gatewayInfo",
		"--json",
		"--field", "response.status",
	}); err != nil {
		t.Fatalf("call --op failed: %v", err)
	}

	if out.String() != "200\n" {
		t.Fatalf("unexpected field output %q", out.String())
	}
	if !sawOpenAPI {
		t.Fatalf("expected OpenAPI auto-sync request")
	}
	if !sawGatewayInfo {
		t.Fatalf("expected gateway info request")
	}
}
