package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestLogsListWrapper(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotPath string
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
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
		"logs", "list",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--query", "minLevel=INFO",
	}); err != nil {
		t.Fatalf("logs list failed: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/data/api/v1/logs" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if gotQuery != "minLevel=INFO" {
		t.Fatalf("unexpected query %q", gotQuery)
	}
}

func TestDiagnosticsBundleGenerateWrapper(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"IN_PROGRESS"}`))
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
		"diagnostics", "bundle", "generate",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--yes",
	}); err != nil {
		t.Fatalf("diagnostics bundle generate failed: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/data/api/v1/diagnostics/bundle/generate" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
}

func TestDiagnosticsBundleGenerateWrapperRequiresYes(t *testing.T) {
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
		"diagnostics", "bundle", "generate",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
	})
	if err == nil {
		t.Fatalf("expected --yes validation failure")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestBackupRestoreWrapper(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotPath string
	var gotBody string
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	inPath := filepath.Join(dir, "backup.gwbk")
	if err := os.WriteFile(inPath, []byte("backup-bytes"), 0o600); err != nil {
		t.Fatalf("write backup fixture: %v", err)
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

	if err := c.Execute([]string{
		"backup", "restore",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--in", inPath,
		"--yes",
	}); err != nil {
		t.Fatalf("backup restore failed: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/data/api/v1/backup" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if gotBody != "backup-bytes" {
		t.Fatalf("unexpected backup body %q", gotBody)
	}
	if gotContentType != "application/octet-stream" {
		t.Fatalf("unexpected content type %q", gotContentType)
	}
}

func TestTagsExportWrapperWritesFile(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotPath string
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tags":[]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "tags.json")

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
		"tags", "export",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--provider", "default",
		"--type", "json",
		"--path", "MyFolder",
		"--out", outPath,
	}); err != nil {
		t.Fatalf("tags export failed: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/data/api/v1/tags/export" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotQuery, "provider=default") || !strings.Contains(gotQuery, "type=json") {
		t.Fatalf("unexpected tags query %q", gotQuery)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
}

func TestRestartGatewayWrapperRequiresYes(t *testing.T) {
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
		"restart", "gateway",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
	})
	if err == nil {
		t.Fatalf("expected --yes validation failure")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestRestartGatewayWrapperSetsConfirmQuery(t *testing.T) {
	t.Parallel()

	var gotQuery string
	var gotPath string
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
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
		"restart", "gateway",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--yes",
	}); err != nil {
		t.Fatalf("restart gateway failed: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/data/api/v1/restart-tasks/restart" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if gotQuery != "confirm=true" {
		t.Fatalf("unexpected query %q", gotQuery)
	}
}
