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

func TestContentParametersWireCLI(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, location := range []string{"query", "path"} {
			for _, tc := range []struct{ media, schema, value, invalid string }{
				{"application/json", `{"type":"object","required":["id","text"],"properties":{"id":{"type":"integer","minimum":9007199254740993},"text":{"type":"string"}},"additionalProperties":false}`, ` {"id":9007199254740993,"text":"a/b+%2F?&= 日本"} `, `{"id":9007199254740992,"text":"private-invalid-value"}`},
				{"text/plain", `{"type":"string","enum":[" text/+%2F?&= 日本 "]}`, " text/+%2F?&= 日本 ", "private-invalid-value"},
			} {
				t.Run(version+"/"+location+"/"+tc.media, func(t *testing.T) {
					template, wirePath, wireQuery, flag := "/inputs", "/proxy/inputs", "", "--query"
					if location == "path" {
						template += "/{value}"
						wirePath += "/" + url.PathEscape(tc.value)
						flag = "--path-param"
					} else {
						wireQuery = url.Values{"value": {tc.value}}.Encode()
					}
					spec := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Synthetic content inputs","version":"test"},"servers":[{"url":"https://foreign.invalid"}],"paths":{%q:{"post":{"parameters":[{"in":%q,"name":"value","required":true,"content":{%q:{"schema":%s}}}],"responses":{"200":{"description":"OK"}}}}}}`, version, template, location, tc.media, tc.schema)
					var writes atomic.Int32
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/proxy/openapi.json" {
							_, _ = io.WriteString(w, spec)
							return
						}
						writes.Add(1)
						if r.Method != "POST" || r.URL.EscapedPath() != wirePath || r.URL.RawQuery != wireQuery || r.Header.Get("X-Ignition-API-Token") != "private-token" {
							t.Error("structured input wire bytes, explicit target, or credential changed")
						}
						_, _ = io.WriteString(w, `{"accepted":true}`)
					}))
					defer srv.Close()
					app, out, stderr := testApp(t, srv)
					args := func(value, mode string) []string {
						return []string{"api", "request", "POST " + template, flag, "value=" + value, mode, "--json"}
					}
					if err := app.Run(context.Background(), args(tc.value, "--dry-run")); err != nil {
						t.Fatal(err)
					}
					preview := decodeResult(t, out)
					if preview.Outcome != "preview" || writes.Load() != 0 || preview.Data.(map[string]any)["validation"] != "declared_schema" {
						t.Fatal("preview dispatched or lost schema coverage")
					}
					if err := app.Run(context.Background(), args(tc.value, "--yes")); err != nil {
						t.Fatal(err)
					}
					if !decodeResult(t, out).OK || writes.Load() != 1 {
						t.Fatal("valid content input did not execute once")
					}
					for _, mode := range []string{"--dry-run", "--yes"} {
						err := app.Run(context.Background(), args(tc.invalid, mode))
						output := out.String() + stderr.String()
						got := decodeResult(t, out)
						if err == nil || got.Error == nil || got.Error.Code != 2 || got.Error.Kind != "validation" || writes.Load() != 1 {
							t.Fatal("invalid content input reached transport or changed error signaling")
						}
						if strings.Contains(output, "private-invalid-value") || strings.Contains(output, "private-token") {
							t.Fatal("validation error disclosed parameter data or credentials")
						}
					}
				})
			}
		}
	}
}
