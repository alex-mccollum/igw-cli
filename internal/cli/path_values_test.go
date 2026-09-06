package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExactPathValuesCLI(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, tt := range []struct{ name, schema, value, invalid string }{
			{"string", `{"type":"string","enum":["a/b + ? # % = 日本"]}`, "a/b + ? # % = 日本", "private-invalid-value"},
			{"number", `{"type":"number","minimum":9007199254740993}`, "9007199254740993", "9007199254740992"},
			{"boolean", `{"type":"boolean","enum":[false]}`, "false", "true"},
		} {
			t.Run(version+"/"+tt.name, func(t *testing.T) {
				const template = "/prefix/items/{value}"
				spec := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Exact paths","version":"test"},"servers":[{"url":"https://foreign.invalid/prefix"}],"paths":{%q:{"post":{"parameters":[{"in":"path","name":"value","required":true,"schema":%s}],"responses":{"200":{"description":"OK"}}}}}}`, version, template, tt.schema)
				var writes atomic.Int32
				wirePath := "/proxy/prefix/items/" + url.PathEscape(tt.value)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/proxy/openapi.json" {
						_, _ = io.WriteString(w, spec)
						return
					}
					writes.Add(1)
					if r.Method != "POST" || r.URL.EscapedPath() != wirePath || r.Header.Get("X-Ignition-API-Token") != "private-token" {
						t.Error("path, target prefix, or managed credential changed")
					}
					_, _ = io.WriteString(w, `{"accepted":true}`)
				}))
				defer srv.Close()
				app, out, _ := testApp(t, srv)
				args := func(value, mode string) []string {
					return []string{"api", "request", "POST " + template, "--path-param", "value=" + value, mode, "--json"}
				}
				if err := app.Run(context.Background(), args(tt.value, "--dry-run")); err != nil {
					t.Fatal(err)
				}
				preview := decodeResult(t, out)
				if preview.Outcome != "preview" || writes.Load() != 0 || preview.Data.(map[string]any)["path"] != strings.TrimPrefix(wirePath, "/proxy") {
					t.Fatal("preview changed path or dispatched a mutation")
				}
				if err := app.Run(context.Background(), args(tt.value, "--yes")); err != nil {
					t.Fatal(err)
				}
				if !decodeResult(t, out).OK || writes.Load() != 1 {
					t.Fatal("valid path did not execute exactly once")
				}
				for _, mode := range []string{"--dry-run", "--yes"} {
					err := app.Run(context.Background(), args(tt.invalid, mode))
					got := decodeResult(t, out)
					if err == nil || got.Error == nil || got.Error.Code != 2 || got.Error.Kind != "validation" || writes.Load() != 1 {
						t.Fatal("invalid path reached transport or changed error contract")
					}
				}
			})
		}
	}
}
