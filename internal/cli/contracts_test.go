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

func TestCallJSONSelectRawExtraction(t *testing.T) {
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
		"--path", "/data/api/v1/gateway-info",
		"--json",
		"--select", "response.status",
		"--raw",
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if out.String() != "200\n" {
		t.Fatalf("unexpected field output %q", out.String())
	}
}

func TestCallJSONSelectRawExtractionFromErrorEnvelope(t *testing.T) {
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
		"--path", "/data/api/v1/gateway-info",
		"--json",
		"--select", "code",
		"--raw",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}
	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if out.String() != "6\n" {
		t.Fatalf("unexpected field output %q", out.String())
	}
}

func TestCallJSONSelectSubsetFromErrorEnvelope(t *testing.T) {
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
		"--path", "/data/api/v1/gateway-info",
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
	if !strings.Contains(payload["error"].(string), "http 403") {
		t.Fatalf("unexpected subset error %#v", payload["error"])
	}
}

func TestCallJSONSelectSubsetExtraction(t *testing.T) {
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
		"--path", "/data/api/v1/gateway-info",
		"--json",
		"--select", "ok",
		"--select", "response.status",
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode fields json: %v", decodeErr)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected ok value %#v", payload["ok"])
	}
	if int(payload["response.status"].(float64)) != 200 {
		t.Fatalf("unexpected response.status %#v", payload["response.status"])
	}
}

func TestCallJSONCompactOutput(t *testing.T) {
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
		"--path", "/data/api/v1/gateway-info",
		"--json",
		"--compact",
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	output := out.String()
	if !json.Valid([]byte(output)) {
		t.Fatalf("expected valid compact json, got %q", output)
	}
	if strings.Contains(output, "\n  ") {
		t.Fatalf("expected compact json without indentation, got %q", output)
	}
}

func TestConfigSetJSONContractUsageError(t *testing.T) {
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
		"config", "set", "--json",
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

func TestAPIShowJSONContractUsageError(t *testing.T) {
	t.Parallel()

	specPath := writeAPISpec(t, apiSpecFixture)
	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{
		"api", "show",
		"--spec-file", specPath,
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
