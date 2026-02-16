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

func TestWaitGatewayRetriesUntilHealthy(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		calls++
		if calls < 2 {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"gateway"}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := testWaitCLI(t, srv, &out)

	if err := c.Execute([]string{
		"wait", "gateway",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "300ms",
	}); err != nil {
		t.Fatalf("wait gateway failed: %v", err)
	}

	if calls < 2 {
		t.Fatalf("expected at least 2 gateway checks, got %d", calls)
	}
	if !strings.Contains(out.String(), "ready\tgateway\thealthy") {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestWaitDiagnosticsBundleReady(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/diagnostics/bundle/status" {
			http.NotFound(w, r)
			return
		}
		calls++
		w.WriteHeader(http.StatusOK)
		if calls < 2 {
			_, _ = w.Write([]byte(`{"state":"IN_PROGRESS","fileSize":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"state":"COMPLETE","fileSize":10}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := testWaitCLI(t, srv, &out)

	if err := c.Execute([]string{
		"wait", "diagnostics-bundle",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "300ms",
		"--json",
		"--select", "ready",
		"--raw",
	}); err != nil {
		t.Fatalf("wait diagnostics-bundle failed: %v", err)
	}

	if out.String() != "true\n" {
		t.Fatalf("unexpected field output %q", out.String())
	}
}

func TestWaitRestartTasksClear(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/restart-tasks/pending" {
			http.NotFound(w, r)
			return
		}
		calls++
		w.WriteHeader(http.StatusOK)
		if calls < 2 {
			_, _ = w.Write([]byte(`{"pending":["Module Enabled: com.example"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"pending":[]}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := testWaitCLI(t, srv, &out)

	if err := c.Execute([]string{
		"wait", "restart-tasks",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "300ms",
		"--json",
		"--select", "target",
		"--select", "attempts",
		"--compact",
	}); err != nil {
		t.Fatalf("wait restart-tasks failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["target"] != "restart-tasks" {
		t.Fatalf("unexpected target %#v", payload["target"])
	}
	if int(payload["attempts"].(float64)) < 2 {
		t.Fatalf("expected attempts >= 2, got %#v", payload["attempts"])
	}
}

func TestWaitTimeoutMapsToNetworkExitCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/restart-tasks/pending" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"pending":["still pending"]}`))
	}))
	defer srv.Close()

	c := testWaitCLI(t, srv, new(bytes.Buffer))

	err := c.Execute([]string{
		"wait", "restart-tasks",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "40ms",
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if code := igwerr.ExitCode(err); code != 7 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestWaitSelectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "select requires json",
			args:    []string{"wait", "gateway", "--select", "ready"},
			wantErr: "required: --json when using --select",
		},
		{
			name:    "raw requires json",
			args:    []string{"wait", "gateway", "--select", "ready", "--raw"},
			wantErr: "required: --json when using --raw",
		},
		{
			name:    "raw requires exactly one select",
			args:    []string{"wait", "gateway", "--json", "--select", "ready", "--select", "target", "--raw"},
			wantErr: "required: exactly one --select when using --raw",
		},
		{
			name:    "raw and compact invalid",
			args:    []string{"wait", "gateway", "--json", "--select", "ready", "--raw", "--compact"},
			wantErr: "cannot use --raw with --compact",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
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

			err := c.Execute(tc.args)
			if err == nil {
				t.Fatalf("expected usage error")
			}
			if code := igwerr.ExitCode(err); code != 2 {
				t.Fatalf("unexpected exit code %d", code)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWaitDiagnosticsBundleFailedStateExitsImmediately(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/diagnostics/bundle/status" {
			http.NotFound(w, r)
			return
		}
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"FAILED","fileSize":0}`))
	}))
	defer srv.Close()

	c := testWaitCLI(t, srv, new(bytes.Buffer))

	err := c.Execute([]string{
		"wait", "diagnostics-bundle",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "500ms",
	})
	if err == nil {
		t.Fatalf("expected failure")
	}
	if code := igwerr.ExitCode(err); code != 7 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if calls != 1 {
		t.Fatalf("expected immediate terminal failure, got %d calls", calls)
	}
}

func TestWaitGatewayAuthFailureExitsImmediately(t *testing.T) {
	t.Parallel()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		calls++
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := testWaitCLI(t, srv, new(bytes.Buffer))

	err := c.Execute([]string{
		"wait", "gateway",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "500ms",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}
	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if calls != 1 {
		t.Fatalf("expected no retries on auth failure, got %d calls", calls)
	}
}

func TestWaitErrorEnvelopeSubsetSelection(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := testWaitCLI(t, srv, &out)

	err := c.Execute([]string{
		"wait", "gateway",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "code",
		"--select", "error",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}
	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("unexpected exit code %d", code)
	}

	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if int(payload["code"].(float64)) != 6 {
		t.Fatalf("unexpected subset code %#v", payload["code"])
	}
	if !strings.Contains(payload["error"].(string), "http 401") {
		t.Fatalf("unexpected subset error %#v", payload["error"])
	}
}

func TestWaitInvalidSelectPathReturnsUsageEnvelope(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"gateway"}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := testWaitCLI(t, srv, &out)

	err := c.Execute([]string{
		"wait", "gateway",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "missing.path",
	})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}

	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if int(payload["code"].(float64)) != 2 {
		t.Fatalf("unexpected code %#v", payload["code"])
	}
	if !strings.Contains(payload["error"].(string), "invalid --select path") {
		t.Fatalf("unexpected error %#v", payload["error"])
	}
}

func TestWaitOptionalConditionSuffix(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"gateway"}`))
	}))
	defer srv.Close()

	c := testWaitCLI(t, srv, new(bytes.Buffer))

	if err := c.Execute([]string{
		"wait", "gateway", "healthy",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--interval", "10ms",
		"--wait-timeout", "200ms",
	}); err != nil {
		t.Fatalf("wait gateway healthy failed: %v", err)
	}
}

func TestWaitRejectsUnexpectedConditionSuffix(t *testing.T) {
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
		"wait", "gateway", "ready",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
	})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func testWaitCLI(t *testing.T, srv *httptest.Server, out *bytes.Buffer) *CLI {
	t.Helper()

	return &CLI{
		In:     strings.NewReader(""),
		Out:    out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}
}
