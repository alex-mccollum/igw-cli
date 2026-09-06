package testgateway_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/cli"
	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestTransferObservationCountsActualCLIRequests(t *testing.T) {
	for _, mode := range []string{"default", "custom"} {
		t.Run(mode, func(t *testing.T) {
			var received atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/openapi.json" {
					_, _ = io.WriteString(w, `{"openapi":"3.1.0","info":{"title":"Request observation","version":"test"},"paths":{"/state":{"get":{"responses":{"200":{"description":"OK"}}}}}}`)
					return
				}
				received.Add(1)
				_, _ = io.WriteString(w, "{}")
			}))
			defer srv.Close()
			var observed atomic.Int64
			client := srv.Client()
			if mode == "default" {
				// Session.HTTPClient uses the standard nil-transport convention.
				client = &http.Client{}
			}
			client.Transport = transferTransport{base: client.Transport, operations: &observed}
			var out bytes.Buffer
			app := cli.App{In: strings.NewReader(""), Out: &out, Err: io.Discard, CacheDir: t.TempDir(), HTTP: client, Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) {
				return config.File{GatewayURL: srv.URL, Token: "synthetic-test-token"}, nil
			}}
			if err := app.Run(context.Background(), []string{"api", "request", "GET /state", "--json"}); err != nil {
				t.Fatal(err)
			}
			if observed.Load() != 1 || received.Load() != 1 {
				t.Fatal("observation missed an operation or counted catalog retrieval")
			}
			input := filepath.Join(t.TempDir(), "tags.json")
			if err := os.WriteFile(input, []byte(`{"tags":[{"name":"test"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := app.Run(context.Background(), []string{"tag", "import", "--in", input, "--yes", "--json"}); err == nil {
				t.Fatal("unavailable import succeeded")
			}
			if observed.Load() != 1 || received.Load() != 1 {
				t.Fatal("unavailable workflow sent an operation")
			}
		})
	}
}
