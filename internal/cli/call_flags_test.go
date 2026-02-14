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

func TestCallRetryRejectedForNonIdempotentMethod(t *testing.T) {
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
		"call",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "secret",
		"--method", "POST",
		"--path", "/data/api/v1/scan/projects",
		"--retry", "1",
		"--yes",
	})
	if err == nil {
		t.Fatalf("expected retry validation error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestCallOutWritesResponseBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"saved":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "response.json")

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
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--out", outPath,
	}); err != nil {
		t.Fatalf("call out failed: %v", err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(b) != `{"saved":true}` {
		t.Fatalf("unexpected output file content: %q", string(b))
	}
}

func TestCallUsesSelectedProfileConfig(t *testing.T) {
	t.Parallel()

	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Ignition-API-Token")
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
			return config.File{
				ActiveProfile: "dev",
				Profiles: map[string]config.Profile{
					"dev": {
						GatewayURL: srv.URL,
						Token:      "profile-token",
					},
				},
			}, nil
		},
		HTTPClient: srv.Client(),
	}

	if err := c.Execute([]string{
		"call",
		"--profile", "dev",
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
	}); err != nil {
		t.Fatalf("call with profile failed: %v", err)
	}

	if gotToken != "profile-token" {
		t.Fatalf("expected token from profile, got %q", gotToken)
	}
}
