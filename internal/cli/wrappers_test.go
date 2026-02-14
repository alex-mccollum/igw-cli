package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestGatewayInfoWrapper(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
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
		"gateway", "info",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
	}); err != nil {
		t.Fatalf("gateway info failed: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/data/api/v1/gateway-info" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
}

func TestScanProjectsWrapperRequiresYes(t *testing.T) {
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
		"scan", "projects",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
	})
	if err == nil {
		t.Fatalf("expected --yes requirement failure")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
