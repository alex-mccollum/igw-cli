package nextcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExplicitEmptyBodyCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, media, schema, coverage string
		flags                         []string
	}{
		{"literal", "text/plain", `{"type":"string","enum":[""]}`, "declared_schema", []string{"--body", ""}},
		{"file", "text/plain", `{"type":"string","enum":[""]}`, "declared_schema", []string{"--body", "@" + path}},
		{"stdin", "text/plain", `{"type":"string","enum":[""]}`, "declared_schema", []string{"--body", "-"}},
		{"opaque", "application/zip", "", "declared_transport", []string{"--body", "@" + path}},
		{"binary", "application/octet-stream", `{"type":"string","format":"binary"}`, "declared_transport", []string{"--upload", path}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			media := "{}"
			if tt.schema != "" {
				media = `{"schema":` + tt.schema + `}`
			}
			spec := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Empty input","version":"test"},"paths":{"/body":{"post":{"requestBody":{"required":true,"content":{%q:%s}},"responses":{"200":{"description":"OK"}}}}}}`, tt.media, media)
			var writes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, spec)
					return
				}
				writes.Add(1)
				raw, err := io.ReadAll(r.Body)
				if err != nil || len(raw) != 0 || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Header.Get("Content-Type") != tt.media || r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("empty body changed bytes, media, framing, or credential")
				}
				_, _ = io.WriteString(w, `{"accepted":true}`)
			}))
			defer srv.Close()
			app, out, _ := testApp(t, srv)
			args := append([]string{"api", "request", "POST /body", "--content-type", tt.media, "--json"}, tt.flags...)
			if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
				t.Fatal(err)
			}
			preview := decodeResult(t, out)
			data := preview.Data.(map[string]any)
			emptyHash := sha256.Sum256(nil)
			if preview.Outcome != "preview" || preview.Meta.Validation != tt.coverage || data["bodyPresent"] != true || data["bodySha256"] != hex.EncodeToString(emptyHash[:]) || writes.Load() != 0 {
				t.Fatal("empty input lost presence, identity, coverage, or preview safety")
			}
			if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
				t.Fatal(err)
			}
			if got := decodeResult(t, out); !got.OK || writes.Load() != 1 {
				t.Fatal("explicit empty body was not sent")
			}
			if err := app.Run(context.Background(), []string{"api", "request", "POST /body", "--content-type", tt.media, "--dry-run", "--json"}); err == nil || writes.Load() != 1 {
				t.Fatal("content type alone replaced a required body input")
			}
		})
	}
}

func TestExplicitEmptyBodyCannotBypassOptionalConstraints(t *testing.T) {
	for _, tt := range []struct{ media, schema string }{
		{"text/plain", `{"type":"string","minLength":1}`},
		{"application/json", `{"type":"object"}`},
		{"application/xml", `{"type":"object"}`},
	} {
		var writes atomic.Int32
		spec := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Optional input","version":"test"},"paths":{"/body":{"post":{"requestBody":{"content":{%q:{"schema":%s}}},"responses":{"200":{"description":"OK"}}}}}}`, tt.media, tt.schema)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/openapi.json") {
				_, _ = io.WriteString(w, spec)
				return
			}
			writes.Add(1)
		}))
		defer srv.Close()
		app, out, _ := testApp(t, srv)
		for _, mode := range []string{"--dry-run", "--yes"} {
			args := []string{"api", "request", "POST /body", "--content-type", tt.media, "--body", "", mode, "--json"}
			err := app.Run(context.Background(), args)
			got := decodeResult(t, out)
			if err == nil || got.OK || got.Error == nil || got.Error.Code != 2 || writes.Load() != 0 {
				t.Fatal("optional empty input bypassed decoding or value constraints")
			}
		}
		if err := app.Run(context.Background(), []string{"api", "request", "POST /body", "--dry-run", "--json"}); err != nil {
			t.Fatal(err)
		}
		data := decodeResult(t, out).Data.(map[string]any)
		if data["bodyPresent"] != false || data["bodySha256"] != nil || writes.Load() != 0 {
			t.Fatal("omitted body was confused with an empty representation")
		}
	}
}

func TestExplicitEmptyBodyRawDefaults(t *testing.T) {
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writes.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil || len(raw) != 0 || r.URL.Path != "/proxy/body" || r.ContentLength != 0 || r.Header.Get("Content-Type") != "application/json" {
			t.Error("raw empty body lost its explicit/default input intent")
		}
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	args := []string{"api", "raw", "--method", "POST", "--path", "/body", "--body", "", "--json"}
	if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
		t.Fatal(err)
	}
	preview := decodeResult(t, out)
	if preview.Meta.Validation != "not_requested" || preview.Data.(map[string]any)["bodyPresent"] != true || writes.Load() != 0 {
		t.Fatal("raw empty preview dispatched or gained schema claims")
	}
	if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
		t.Fatal(err)
	}
	if got := decodeResult(t, out); !got.OK || got.Meta.Validation != "not_requested" || writes.Load() != 1 {
		t.Fatal("raw empty execution changed validation or dispatch")
	}
}
