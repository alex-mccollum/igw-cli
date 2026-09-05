package nextcli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExactHeaderValuesCLI(t *testing.T) {
	for _, tt := range []struct {
		name, declaration string
		valid, invalid    []string
	}{
		{"empty string", `"schema":{"type":"string","maxLength":0}`, []string{""}, []string{"private-invalid-value"}},
		{"Unicode whitespace", `"schema":{"type":"string","enum":["\u00a0literal\u00a0"]}`, []string{"\u00a0literal\u00a0"}, []string{"literal"}},
		{"exact number", `"schema":{"type":"number","minimum":9007199254740993}`, []string{"9007199254740993"}, []string{"9007199254740992"}},
		{"whole array", `"schema":{"type":"array","minItems":2,"uniqueItems":true,"items":{"type":"string"}}`, []string{"a%2Cb", "\u00a0literal\u00a0"}, []string{"private-invalid-value", "private-invalid-value"}},
		{"JSON content", `"content":{"application/json":{"schema":{"type":"object","required":["value"],"properties":{"value":{"type":"integer","minimum":9007199254740993}}}}}`, []string{`{"value":9007199254740993}`}, []string{`{"value":9007199254740992,"secret":"private-invalid-value"}`}},
		{"JSON null", `"content":{"application/json":{"schema":{"type":"null"}}}`, []string{"null"}, []string{`"private-invalid-value"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Exact headers","version":"test"},"paths":{"/headers":{"post":{"parameters":[{"in":"header","name":"X-Value","required":true,%s},{"in":"header","name":"X-Ignition-API-Token","required":true,"schema":{"type":"string","enum":["marker-is-not-a-credential"]}}],"responses":{"200":{"description":"OK"}}}}}}`, tt.declaration)
			var writes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, spec)
					return
				}
				writes.Add(1)
				if r.Method != "POST" || r.URL.Path != "/proxy/headers" || !reflect.DeepEqual(r.Header.Values("X-Value"), tt.valid) || r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("header values, target, or managed credential changed")
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			app, out, stderr := testApp(t, srv)
			args := func(values []string, mode string) []string {
				args := []string{"api", "request", "POST /headers", mode, "--json"}
				for _, value := range values {
					args = append(args, "--header", "x-value:"+value)
				}
				return args
			}
			if err := app.Run(context.Background(), args(tt.valid, "--dry-run")); err != nil {
				t.Fatal(err)
			}
			previewOutput := out.String() + stderr.String()
			if decodeResult(t, out).Outcome != "preview" || writes.Load() != 0 || strings.Contains(previewOutput, "private-token") {
				t.Fatal("preview dispatched or exposed a credential")
			}
			if err := app.Run(context.Background(), args(tt.valid, "--yes")); err != nil {
				t.Fatal(err)
			}
			if !decodeResult(t, out).OK || writes.Load() != 1 {
				t.Fatal("valid headers did not execute exactly once")
			}
			for _, mode := range []string{"--dry-run", "--yes"} {
				err := app.Run(context.Background(), args(tt.invalid, mode))
				output := out.String() + stderr.String()
				got := decodeResult(t, out)
				if err == nil || got.Error == nil || got.Error.Code != 2 || got.Error.Kind != "validation" || writes.Load() != 1 {
					t.Fatal("invalid headers reached transport or changed error contract")
				}
				if strings.Contains(output, "private-invalid-value") || strings.Contains(output, "private-token") {
					t.Fatal("validation exposed a header value or credential")
				}
			}
		})
	}
}
