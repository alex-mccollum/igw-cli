package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBinaryBodySchemasStreamBeyondInlineLimit(t *testing.T) {
	const size int64 = 40 << 20
	path := filepath.Join(t.TempDir(), "input.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	for _, schema := range []string{`{}`, `{"type":"string","format":"binary"}`} {
		t.Run(schema, func(t *testing.T) {
			spec := strings.Replace(uploadSpec, `"application/zip":{}`, `"application/octet-stream":{"schema":`+schema+`}`, 1)
			var writes atomic.Int32
			var received atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, spec)
					return
				}
				writes.Add(1)
				if r.Method != "POST" || r.ContentLength != size || r.Header.Get("Content-Type") != "application/octet-stream" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("binary stream framing or credentials changed")
				}
				hash := sha256.New()
				n, err := io.Copy(hash, r.Body)
				if err != nil || n != size {
					t.Error("binary stream was incomplete")
				}
				received.Store(hex.EncodeToString(hash.Sum(nil)))
				_, _ = io.WriteString(w, `{"accepted":true}`)
			}))
			defer srv.Close()
			app, out, _ := testApp(t, srv)
			args := []string{"api", "request", "POST /upload", "--upload", path, "--content-type", "application/octet-stream", "--json"}
			if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
				t.Fatal(err)
			}
			preview := decodeResult(t, out)
			if !preview.OK || preview.Meta.Validation != "declared_transport" || writes.Load() != 0 {
				t.Fatal("binary preview sent a write or claimed value validation")
			}
			if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
				t.Fatal(err)
			}
			got := decodeResult(t, out)
			if !got.OK || got.Meta.Validation != "declared_transport" || writes.Load() != 1 || received.Load() != preview.Data.(map[string]any)["bodySha256"] {
				t.Fatal("binary upload changed the reviewed bytes or validation coverage")
			}
		})
	}
}
