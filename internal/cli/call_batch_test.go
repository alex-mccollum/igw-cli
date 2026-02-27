package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestCallBatchJSONOutput(t *testing.T) {
	t.Parallel()

	client := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		return mockHTTPResponse(http.StatusOK, `{"ok":true}`, nil), nil
	})

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
		HTTPClient: client,
	}

	if err := c.Execute([]string{
		"call",
		"--gateway-url", mockGatewayURL,
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

	client := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/missing" {
			return mockHTTPResponse(http.StatusNotFound, `{"error":"missing"}`, nil), nil
		}
		return mockHTTPResponse(http.StatusOK, `{"ok":true}`, nil), nil
	})

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
		HTTPClient: client,
	}

	err := c.Execute([]string{
		"call",
		"--gateway-url", mockGatewayURL,
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

func TestRunCallBatchParallelExecutesConcurrently(t *testing.T) {
	t.Parallel()

	var active int32
	var maxActive int32
	client := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			recorded := atomic.LoadInt32(&maxActive)
			if current <= recorded {
				break
			}
			if atomic.CompareAndSwapInt32(&maxActive, recorded, current) {
				break
			}
		}
		defer atomic.AddInt32(&active, -1)

		// Add meaningful runtime so overlap can be detected.
		time.Sleep(40 * time.Millisecond)
		return mockHTTPResponse(http.StatusOK, `{"ok":true}`, nil), nil
	})

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: client,
	}

	dir := t.TempDir()
	batchFile := filepath.Join(dir, "batch.ndjson")
	items := make([]string, 6)
	for i := 0; i < len(items); i++ {
		items[i] = `{"id":"item-` + string(rune('a'+i)) + `","method":"GET","path":"/data/api/v1/gateway-info"}`
	}
	if err := os.WriteFile(batchFile, []byte(strings.Join(items, "\n")), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}

	err := c.runCallBatch(mockGatewayURL, "secret", "@"+batchFile, callBatchDefaults{
		Retry:        0,
		RetryBackoff: 250 * time.Millisecond,
		Timeout:      2 * time.Second,
		Yes:          false,
		OutputFormat: "json",
		Parallel:     3,
	})
	if err != nil {
		t.Fatalf("runCallBatch failed: %v", err)
	}

	if atomic.LoadInt32(&maxActive) < 2 {
		t.Fatalf("expected at least 2 concurrent in-flight requests, got %d", atomic.LoadInt32(&maxActive))
	}

	var payload []callBatchItemResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(payload) != len(items) {
		t.Fatalf("expected %d results, got %d", len(items), len(payload))
	}
	for i, got := range payload {
		want := "item-" + string(rune('a'+i))
		if got.ID != want {
			t.Fatalf("expected payload[%d].ID=%q, got %#v", i, want, got.ID)
		}
	}
}

func TestRunCallBatchParallelHandlesLargeInputWithoutDeadlock(t *testing.T) {
	t.Parallel()

	client := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		return mockHTTPResponse(http.StatusOK, `{"ok":true}`, nil), nil
	})

	dir := t.TempDir()
	batchFile := filepath.Join(dir, "batch.ndjson")
	items := make([]string, 1500)
	for i := 0; i < len(items); i++ {
		items[i] = `{"method":"GET","path":"/data/api/v1/gateway-info"}`
	}
	if err := os.WriteFile(batchFile, []byte(strings.Join(items, "\n")), 0o600); err != nil {
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
		HTTPClient: client,
	}

	resultChan := make(chan error, 1)
	go func() {
		resultChan <- c.runCallBatch(mockGatewayURL, "secret", "@"+batchFile, callBatchDefaults{
			Retry:        0,
			RetryBackoff: 250 * time.Millisecond,
			Timeout:      2 * time.Second,
			Yes:          false,
			OutputFormat: "ndjson",
			Parallel:     8,
		})
	}()

	select {
	case err := <-resultChan:
		if err != nil {
			t.Fatalf("runCallBatch failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runCallBatch did not complete in time")
	}

	lines := strings.Count(out.String(), "\n")
	if lines != len(items) {
		t.Fatalf("expected %d ndjson lines, got %d", len(items), lines)
	}
}
