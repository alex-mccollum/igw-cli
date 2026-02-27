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
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestCallBatchJSONOutput(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	batchFile := filepath.Join(dir, "batch.ndjson")
	content := strings.Join([]string{
		`{"id":"a","method":"GET","path":"/data/api/v1/gateway-info"}`,
		`{"id":"b","method":"GET","path":"/data/api/v1/gateway-info"}`,
	}, "\n")
	if err := os.WriteFile(batchFile, []byte(content), 0o600); err != nil {
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
		t.Fatalf("decode output: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 results, got %d", len(payload))
	}
	if payload[0]["ok"] != true || payload[1]["ok"] != true {
		t.Fatalf("expected successful batch results: %#v", payload)
	}
}

func TestCallBatchAggregatesExitCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"missing"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	batchFile := filepath.Join(dir, "batch.ndjson")
	content := strings.Join([]string{
		`{"id":"ok","method":"GET","path":"/data/api/v1/gateway-info"}`,
		`{"id":"missing","method":"GET","path":"/missing"}`,
	}, "\n")
	if err := os.WriteFile(batchFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}

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

	err := c.Execute([]string{
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--batch", "@" + batchFile,
	})
	if err == nil {
		t.Fatalf("expected aggregated batch error")
	}
	if code := igwerr.ExitCode(err); code != 7 {
		t.Fatalf("expected network exit code 7, got %d", code)
	}
}

func TestReadCallBatchItemsSupportsJSONArray(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	batchFile := filepath.Join(tempDir, "batch.json")
	content := `[
		{"id":"a","method":"GET","path":"/data/api/v1/gateway-info"},
		{"id":"b","operationId":"gatewayInfo"}
	]`
	if err := os.WriteFile(batchFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}

	items, err := readCallBatchItems(strings.NewReader(""), "@"+batchFile)
	if err != nil {
		t.Fatalf("read batch items failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "a" {
		t.Fatalf("expected first id a, got %#v", items[0].ID)
	}
}

func TestReadCallBatchItemsRejectsEmptyArray(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	batchFile := filepath.Join(tempDir, "batch-empty.json")
	if err := os.WriteFile(batchFile, []byte("[]"), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}

	_, err := readCallBatchItems(strings.NewReader(""), "@"+batchFile)
	if err == nil {
		t.Fatalf("expected empty array usage error")
	}
}
