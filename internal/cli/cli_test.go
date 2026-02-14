package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestCallTimeoutMapsToNetworkExitCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    &errOut,
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
		"--timeout", "10ms",
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}

	if code := igwerr.ExitCode(err); code != 7 {
		t.Fatalf("exit code: got %d want 7", code)
	}
}

func TestCallBodyVariants(t *testing.T) {
	t.Parallel()

	t.Run("raw", func(t *testing.T) {
		t.Parallel()

		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := new(bytes.Buffer)
			if _, err := body.ReadFrom(r.Body); err != nil {
				t.Fatalf("read body: %v", err)
			}
			gotBody = body.String()
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

		err := c.Execute([]string{
			"call",
			"--gateway-url", srv.URL,
			"--api-key", "secret",
			"--method", "POST",
			"--path", "/data/api/v1/scan/projects",
			"--body", `{"raw":true}`,
			"--yes",
		})
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if gotBody != `{"raw":true}` {
			t.Fatalf("body: got %q", gotBody)
		}
	})

	t.Run("file", func(t *testing.T) {
		t.Parallel()

		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := new(bytes.Buffer)
			if _, err := body.ReadFrom(r.Body); err != nil {
				t.Fatalf("read body: %v", err)
			}
			gotBody = body.String()
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		dir := t.TempDir()
		bodyPath := filepath.Join(dir, "body.json")
		if err := os.WriteFile(bodyPath, []byte(`{"file":true}`), 0o600); err != nil {
			t.Fatalf("write body file: %v", err)
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
			"--method", "POST",
			"--path", "/data/api/v1/scan/projects",
			"--body", "@" + bodyPath,
			"--yes",
		})
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if gotBody != `{"file":true}` {
			t.Fatalf("body: got %q", gotBody)
		}
	})
}
