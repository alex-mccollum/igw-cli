package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestAPIBodyValidationCoverageAndRefusals(t *testing.T) {
	const spec = `{"openapi":"3.1.0","info":{"title":"Synthetic CLI body coverage","version":"test"},"paths":{"/body":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["enabled"],"properties":{"enabled":{"type":"boolean"}}}},"text/plain":{"schema":{"type":"string","enum":[" exact + 日本 \n"]}},"application/zip":{},"application/xml":{"schema":{"type":"object"}}}},"responses":{"200":{"description":"OK"}}}}}}`
	var writes atomic.Int32
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		writes.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil || r.Method != "POST" || r.URL.Path != "/proxy/body" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("body request framing, route, or credentials changed")
		}
		received.Store([2]string{r.Header.Get("Content-Type"), string(raw)})
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	args := func(media, body, mode string) []string {
		return []string{"api", "request", "POST /body", "--content-type", media, "--body", body, mode, "--json"}
	}
	for _, tt := range []struct{ media, body, coverage string }{
		{"application/json", `{"enabled":false}`, "declared_schema"},
		{"text/plain", " exact + 日本 \n", "declared_schema"},
		{"application/zip", "\x00\xff", "declared_transport"},
		{"application/zip", "", "declared_transport"},
	} {
		before := writes.Load()
		if err := app.Run(context.Background(), args(tt.media, tt.body, "--dry-run")); err != nil {
			t.Fatal(err)
		}
		preview := decodeResult(t, out)
		if !preview.OK || preview.Outcome != "preview" || preview.Meta.Validation != tt.coverage || preview.Data.(map[string]any)["validation"] != tt.coverage || writes.Load() != before {
			t.Fatal("preview misreported coverage or dispatched a write")
		}
		if err := app.Run(context.Background(), args(tt.media, tt.body, "--yes")); err != nil {
			t.Fatal(err)
		}
		got := decodeResult(t, out)
		if !got.OK || got.Meta.Validation != tt.coverage || writes.Load() != before+1 || received.Load() != [2]string{tt.media, tt.body} {
			t.Fatal("execution lost body coverage or changed bytes")
		}
	}
	before := writes.Load()
	for _, tt := range []struct{ media, body, kind string }{
		{"text/plain", "private-body-text", "validation"},
		{"application/json", `{"enabled":"private-body-text"}`, "validation"},
		{"application/xml", `<secret>private-body-text</secret>`, "unsupported_input"},
		{"text/plain; charset=iso-8859-1", "private-body-text", "unsupported_input"},
	} {
		for _, mode := range []string{"--dry-run", "--yes"} {
			err := app.Run(context.Background(), args(tt.media, tt.body, mode))
			if strings.Contains(out.String(), "private-body-text") || strings.Contains(out.String(), "private-token") || stderr.Len() != 0 {
				t.Fatal("body refusal exposed values or credentials")
			}
			got := decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || got.OK || got.Error == nil || got.Error.Kind != tt.kind || writes.Load() != before {
				t.Fatalf("body refusal lost its cause or dispatched: %+v %v", got, err)
			}
		}
	}
	if err := app.Run(context.Background(), []string{"api", "raw", "--method", "POST", "--path", "/body", "--content-type", "application/xml", "--body", "<root/>", "--yes", "--json"}); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	if !got.OK || got.Meta.Validation != "not_requested" || writes.Load() != before+1 {
		t.Fatal("explicit raw request acquired schema-validation claims")
	}
}
