package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestCallStreamRespectsMaxBodyBytes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "payload.txt")

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
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--path", "/data/api/v1/gateway-info",
		"--stream",
		"--max-body-bytes", "3",
		"--out", outPath,
	}); err != nil {
		t.Fatalf("stream call failed: %v", err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(b) != "abc" {
		t.Fatalf("unexpected streamed body %q", string(b))
	}
}
