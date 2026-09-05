package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func artifactTestCLI(srv *httptest.Server, out *bytes.Buffer) *CLI {
	return &CLI{
		In: strings.NewReader(""), Out: out, Err: new(bytes.Buffer),
		Getenv:     func(string) string { return "" },
		ReadConfig: func() (config.File, error) { return config.File{}, nil },
		HTTPClient: srv.Client(),
	}
}

func TestCallJSONOutSavesArtifactAndReportsChecksum(t *testing.T) {
	t.Parallel()
	body := "\x00\xffbinary archive"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "backup.gwbk")
	out := new(bytes.Buffer)
	c := artifactTestCLI(srv, out)
	err := c.Execute([]string{"backup", "export", "--gateway-url", srv.URL, "--api-key", "secret", "--json", "--out", path})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != body {
		t.Fatalf("artifact: %q, %v", data, err)
	}
	var result callJSONEnvelope
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	info := result.Response.Artifact
	if info == nil || info.Path != path || info.Bytes != int64(len(body)) || info.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
		t.Fatalf("missing artifact evidence: %s", out)
	}
	if result.Response.Body != "" {
		t.Fatal("binary artifact was copied into JSON")
	}
}

func TestFailedDownloadsPreserveExistingArtifacts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		short  bool
		flags  []string
		code   int
	}{
		{name: "http", status: 401, code: 6},
		{name: "truncation", flags: []string{"--max-body-bytes", "3"}, code: 7},
		{name: "broken stream", short: true, code: 7},
		{name: "missing body", flags: []string{"--body", "@nonexistent-input"}, code: 2},
		{name: "confirmation", flags: []string{"--method", "POST"}, code: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.short {
					w.Header().Set("Content-Length", "100")
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = w.Write([]byte("partial"))
			}))
			defer srv.Close()
			dir := t.TempDir()
			path := filepath.Join(dir, "backup")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			c := artifactTestCLI(srv, new(bytes.Buffer))
			args := []string{"call", "--gateway-url", srv.URL, "--api-key", "secret", "--path", "/download", "--out", path, "--overwrite", "--json"}
			err := c.Execute(append(args, tc.flags...))
			if igwerr.ExitCode(err) != tc.code {
				t.Fatalf("wrong failure: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "original" {
				t.Fatalf("lost original: %q, %v", data, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary files remain: %v, %v", entries, err)
			}
		})
	}
}

func TestDownloadRequiresExplicitOverwriteBeforeRequest(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("replacement"))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "backup")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	c := artifactTestCLI(srv, new(bytes.Buffer))
	args := []string{"backup", "export", "--gateway-url", srv.URL, "--api-key", "secret", "--out", path}
	if err := c.Execute(args); igwerr.ExitCode(err) != 2 {
		t.Fatalf("missing overwrite guard: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("request sent before overwrite validation")
	}
	if err := c.Execute(append(args, "--overwrite")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("replacement: %q, %v", data, err)
	}
}
