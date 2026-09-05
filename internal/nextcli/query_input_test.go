package nextcli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestQueryInputNamesCLI(t *testing.T) {
	for _, name := range []string{" name ", "\u00a0name\u00a0", " ", "+&%#[]日本"} {
		t.Run(name, func(t *testing.T) {
			spec := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Query names","version":"test"},"paths":{"/query":{"post":{"parameters":[{"in":"query","name":%q,"required":true,"schema":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"string"}}}],"responses":{"200":{"description":"OK"}}}}}}`, name)
			want := url.Values{name: {"one=value", " + & % # / = 日本 "}}
			var writes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, spec)
					return
				}
				writes.Add(1)
				if !reflect.DeepEqual(r.URL.Query(), want) || r.URL.Path != "/proxy/query" {
					t.Error("CLI validation and transport used different query names")
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			app, out, _ := testApp(t, srv)
			for _, prefix := range [][]string{{"api", "request", "POST /query"}, {"api", "raw", "--method", "POST", "--path", "/query"}} {
				args := append(append([]string{}, prefix...), "--query", name+"=one=value", "--query", name+"= + & % # / = 日本 ", "--json")
				before := writes.Load()
				if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
					t.Fatal(err)
				}
				preview := decodeResult(t, out)
				if preview.Outcome != "preview" || writes.Load() != before || !reflect.DeepEqual(preview.Data.(map[string]any)["queryKeys"], []any{name}) {
					t.Fatal("query preview changed names or dispatched")
				}
				if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
					t.Fatal(err)
				}
				if !decodeResult(t, out).OK || writes.Load() != before+1 {
					t.Fatal("query execution failed")
				}
			}
		})
	}
}
