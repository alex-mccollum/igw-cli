package nextcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const uploadSpec = `{"openapi":"3.1.0","info":{"title":"Upload test","version":"test"},"paths":{"/upload":{"post":{"requestBody":{"required":true,"content":{"application/zip":{}}},"responses":{"200":{"description":"OK"}}}}}}`

func TestLargeUploadStreamsExactBytesAndPreviewsWithoutDispatch(t *testing.T) {
	const size int64 = 40 << 20 // Exceeds the bounded in-memory request-body limit.
	path := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	f.Close()
	var writes atomic.Int32
	var observed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, uploadSpec)
			return
		}
		writes.Add(1)
		if r.Method != "POST" || r.ContentLength != size || r.Header.Get("Content-Type") != "application/zip" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("upload framing or credentials changed")
		}
		h := sha256.New()
		n, err := io.Copy(h, r.Body)
		if n != size || err != nil {
			t.Error("stream incomplete")
		}
		observed = hex.EncodeToString(h.Sum(nil))
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	args := []string{"api", "request", "POST /upload", "--upload", path, "--content-type", "application/zip", "--json"}
	if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
		t.Fatal(err)
	}
	preview := decodeResult(t, out)
	data := preview.Data.(map[string]any)
	if preview.Outcome != "preview" || data["validation"] != "declared_transport" || data["bodyBytes"] != json.Number("41943040") || writes.Load() != 0 {
		t.Fatalf("incorrect upload preview: %+v", preview)
	}
	if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	if !got.OK || writes.Load() != 1 || observed != data["bodySha256"] {
		t.Fatal("streamed bytes differ from preview")
	}
}

func TestUploadCannotBypassSchemaOrOverrideFraming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("private-content"), 0600); err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			spec := strings.Replace(uploadSpec, `"application/zip":{}`, `"application/json":{"schema":{"type":"object","required":["enabled"]}}`, 1)
			_, _ = io.WriteString(w, spec)
			return
		}
		writes.Add(1)
	}))
	defer srv.Close()
	for _, args := range [][]string{
		{"api", "request", "POST /upload", "--upload", path, "--content-type", "application/json", "--yes"},
		{"api", "raw", "--path", "/upload", "--upload", path, "--body", "x", "--content-type", "application/zip", "--yes"},
		{"api", "raw", "--path", "/upload", "--upload", path, "--yes"},
		{"api", "raw", "--method", "POST", "--path", "/upload", "--upload", path, "--content-type", "application/zip", "--header", "Content-Length: 1", "--yes"},
	} {
		app, out, _ := testApp(t, srv)
		err := app.Run(context.Background(), append(args, "--json"))
		if strings.Contains(out.String(), "private-content") {
			t.Fatal("upload contents leaked")
		}
		got := decodeResult(t, out)
		if err == nil || got.OK || got.Error.Code != 2 {
			t.Fatalf("unsafe upload accepted: %+v", got)
		}
	}
	if writes.Load() != 0 {
		t.Fatal("rejected upload reached Gateway")
	}
}
