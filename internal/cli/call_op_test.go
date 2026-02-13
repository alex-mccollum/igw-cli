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
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const callOpSpecFixture = `{
  "openapi": "3.0.0",
  "paths": {
    "/data/api/v1/gateway-info": {
      "get": {
        "operationId": "gatewayInfo",
        "summary": "Gateway info"
      }
    },
    "/data/api/v1/scan/projects": {
      "post": {
        "operationId": "scanProjects",
        "summary": "Scan projects"
      }
    }
  }
}`

func TestCallOperationIDResolvesMethodAndPath(t *testing.T) {
	t.Parallel()

	specPath := writeCallOpSpec(t, callOpSpecFixture)

	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
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

	err := c.Execute([]string{
		"call",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--op", "gatewayInfo",
		"--spec-file", specPath,
	})
	if err != nil {
		t.Fatalf("call by op failed: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("unexpected method %q", gotMethod)
	}
	if gotPath != "/data/api/v1/gateway-info" {
		t.Fatalf("unexpected path %q", gotPath)
	}
}

func TestCallOperationIDConflictWithMethodAndPath(t *testing.T) {
	t.Parallel()

	specPath := writeCallOpSpec(t, callOpSpecFixture)

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
		"call",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
		"--op", "gatewayInfo",
		"--spec-file", specPath,
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
	})
	if err == nil {
		t.Fatalf("expected conflict error")
	}

	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestCallOperationIDNotFound(t *testing.T) {
	t.Parallel()

	specPath := writeCallOpSpec(t, callOpSpecFixture)

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
		"call",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
		"--op", "doesNotExist",
		"--spec-file", specPath,
	})
	if err == nil {
		t.Fatalf("expected not found error")
	}

	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func writeCallOpSpec(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write spec fixture: %v", err)
	}
	return path
}
