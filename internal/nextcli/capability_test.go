package nextcli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

func tagCapabilitySpec(imports, exports bool) string {
	paths := map[string]any{}
	for route, present := range map[string]bool{"import": imports, "export": exports} {
		if !present {
			continue
		}
		params := []map[string]any{}
		for _, name := range []string{"provider", "type", "path", "collisionPolicy", "recursive", "includeUdts"} {
			params = append(params, map[string]any{"name": name, "in": "query", "schema": map[string]any{"type": "string"}})
		}
		op := map[string]any{"parameters": params, "responses": map[string]any{"200": map[string]any{"description": "OK"}}}
		method := "get"
		if route == "import" {
			method = "post"
			op["requestBody"] = map[string]any{"content": map[string]any{"application/octet-stream": map[string]any{}}}
		}
		paths["/data/api/v1/tags/"+route] = map[string]any{method: op}
	}
	data, _ := json.Marshal(map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Synthetic tag capabilities", "version": "irrelevant-version"}, "paths": paths})
	return string(data)
}

func TestTagCapabilitiesRefuseBeforeDispatchAndArtifact(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		imports, exports, preview bool
		format, action            string
		accepted                  bool
	}{
		{name: "missing both", format: "json", action: "import"},
		{name: "missing readback", imports: true, format: "json", action: "import"},
		{name: "preview missing readback", imports: true, preview: true, format: "json", action: "import"},
		{name: "missing write", exports: true, format: "json", action: "import"},
		{name: "opaque acknowledgement", imports: true, format: "xml", action: "import", accepted: true},
		{name: "missing export", format: "json", action: "export"},
		{name: "available export", exports: true, format: "xml", action: "export", accepted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, tagCapabilitySpec(tc.imports, tc.exports))
					return
				}
				calls.Add(1)
				if r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("workflow lost target-bound credentials")
				}
				if tc.action == "export" {
					if r.Method != "GET" || r.URL.Path != "/proxy/data/api/v1/tags/export" || r.URL.Query().Get("provider") != "default" || r.URL.Query().Get("recursive") != "true" || r.URL.Query().Get("includeUdts") != "true" || r.URL.Query().Get("type") != "xml" {
						t.Error("typed export changed its request")
					}
					_, _ = io.WriteString(w, "<Tags/>")
				} else {
					_, _ = io.WriteString(w, "[]")
				}
			}))
			defer srv.Close()
			app, out, _ := testApp(t, srv)
			path := filepath.Join(t.TempDir(), "tags."+tc.format)
			args := []string{"tag", tc.action, "--type", tc.format, "--json"}
			if tc.action == "export" {
				args = append(args, "--out", path)
			} else {
				body := `{"tags":[{"name":"test"}]}`
				if tc.format == "xml" {
					body = "<Tags/>"
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				confirmation := "--yes"
				if tc.preview {
					confirmation = "--dry-run"
				}
				args = append(args, "--in", path, confirmation)
			}
			err := app.Run(context.Background(), args)
			got := decodeResult(t, out)
			if tc.accepted {
				if err != nil || !got.OK || calls.Load() != 1 {
					t.Fatalf("available workflow failed: %+v %v", got, err)
				}
				if tc.action == "import" && (got.Outcome != "accepted" || got.Meta.Verification != "unavailable") {
					t.Fatal("opaque import claimed independent verification")
				}
				if tc.action == "export" {
					body, err := os.ReadFile(path)
					if err != nil || string(body) != "<Tags/>" || got.Artifact == nil {
						t.Fatal("export did not preserve streamed artifact")
					}
				}
				return
			}
			if err == nil || got.OK || got.Error.Kind != "capability" || got.Error.Code != 2 || got.Meta.Catalog == nil || got.Meta.HTTPStatus != 0 || calls.Load() != 0 {
				t.Fatalf("missing capability reached execution: %+v %v", got, err)
			}
			if len(got.Error.Details.(map[string]any)["missingOperations"].([]any)) == 0 {
				t.Fatal("missing operations omitted from machine guidance")
			}
			if tc.action == "export" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("unavailable export created an artifact")
				}
			}
		})
	}
}

func TestCapabilityDiscoveryUsesSelectedCatalogAndHumanGuidance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/openapi.json" {
			t.Error("discovery sent an operation")
		}
		_, _ = io.WriteString(w, tagCapabilitySpec(true, false))
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	if err := app.Run(context.Background(), []string{"api", "capabilities", "--json"}); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	var items []catalog.CapabilityAssessment
	b, _ := json.Marshal(got.Data)
	if json.Unmarshal(b, &items) != nil || len(items) != 3 || got.Meta.Catalog == nil || items[0].Status != "unavailable" || items[1].Status != "advertised" || items[2].Status != "unavailable" {
		t.Fatalf("discovery did not use selected operations: %+v", got)
	}
	if err := app.Run(context.Background(), []string{"api", "capabilities", "--offline"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tag.import.verified-json\tunavailable") || !strings.Contains(out.String(), "missing: GET /data/api/v1/tags/export") {
		t.Fatal("human guidance did not identify missing readback")
	}
}
