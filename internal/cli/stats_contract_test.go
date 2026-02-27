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

func TestCallJSONStatsSchemaV1(t *testing.T) {
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
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--path", "/data/api/v1/gateway-info",
		"--json",
		"--json-stats",
	}); err != nil {
		t.Fatalf("call failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode call payload: %v", err)
	}
	stats, ok := payload["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected call stats payload: %#v", payload)
	}
	assertCallStatsContractV1(t, stats)
}

func TestCallBatchStatsSchemaV1(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	batchFile := filepath.Join(dir, "batch.ndjson")
	if err := os.WriteFile(batchFile, []byte(`{"id":"a","method":"GET","path":"/data/api/v1/gateway-info"}`), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}

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
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--batch", "@" + batchFile,
		"--batch-output", "json",
	}); err != nil {
		t.Fatalf("call batch failed: %v", err)
	}

	var payload []map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode batch payload: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected one batch result, got %d", len(payload))
	}
	stats, ok := payload[0]["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected batch stats payload: %#v", payload[0])
	}
	assertCallStatsContractV1(t, stats)
}

func assertCallStatsContractV1(t *testing.T, stats map[string]any) {
	t.Helper()

	version, ok := stats["version"].(float64)
	if !ok || int(version) != callStatsSchemaVersion {
		t.Fatalf("expected stats.version=%d, got %#v", callStatsSchemaVersion, stats["version"])
	}
	timingMs, ok := stats["timingMs"].(float64)
	if !ok || timingMs < 0 {
		t.Fatalf("expected non-negative stats.timingMs, got %#v", stats["timingMs"])
	}
	bodyBytes, ok := stats["bodyBytes"].(float64)
	if !ok || bodyBytes < 0 {
		t.Fatalf("expected non-negative stats.bodyBytes, got %#v", stats["bodyBytes"])
	}
}
