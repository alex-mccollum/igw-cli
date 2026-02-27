package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
)

func BenchmarkExecuteCallCore(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &gateway.Client{
		BaseURL: srv.URL,
		Token:   "secret",
		HTTP:    srv.Client(),
	}

	input := callExecutionInput{
		Method:  "GET",
		Path:    "/data/api/v1/gateway-info",
		Timeout: 2 * time.Second,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _, _, err := executeCallCore(client, input)
		if err != nil {
			b.Fatalf("executeCallCore: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status code %d", resp.StatusCode)
		}
	}
}

func BenchmarkRunCallBatchSingleItem(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := b.TempDir()
	batchFile := filepath.Join(dir, "batch.ndjson")
	if err := os.WriteFile(batchFile, []byte(`{"id":"a","method":"GET","path":"/data/api/v1/gateway-info"}`), 0o600); err != nil {
		b.Fatalf("write batch file: %v", err)
	}

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    io.Discard,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	defaults := callBatchDefaults{
		Retry:        0,
		RetryBackoff: 250 * time.Millisecond,
		Timeout:      2 * time.Second,
		Yes:          false,
		OutputFormat: "json",
		Parallel:     1,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.runCallBatch(srv.URL, "secret", "@"+batchFile, defaults); err != nil {
			b.Fatalf("runCallBatch: %v", err)
		}
	}
}

func BenchmarkHandleRPCCall(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    io.Discard,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	req := rpcRequest{
		ID: "bench-1",
		Op: "call",
		Args: json.RawMessage(`{
			"method":"GET",
			"path":"/data/api/v1/gateway-info"
		}`),
	}
	common := wrapperCommon{
		gatewayURL: srv.URL,
		apiKey:     "secret",
		timeout:    2 * time.Second,
	}
	session := newRPCSessionState()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := c.handleRPCCall(req, common, "openapi.json", session)
		if !resp.OK {
			b.Fatalf("handleRPCCall: %#v", resp)
		}
		if resp.Status != http.StatusOK {
			b.Fatalf("unexpected status code %d", resp.Status)
		}
	}
}
