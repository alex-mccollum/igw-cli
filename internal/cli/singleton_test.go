package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestSingletonWorkflowCLI(t *testing.T) {
	const base = "/data/api/v1/resources/test/settings"
	const read = "/data/api/v1/resources/singleton/test/settings"
	body := func(required string) string {
		return fmt.Sprintf(`{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","minItems":1,"maxItems":1,"items":{"type":"object","properties":{"collection":{"type":"string"},"description":{"type":"string"},"signature":{"type":"string"}},"required":%s,"additionalProperties":false}}}}},"responses":{"200":{"description":"OK"}}}`, required)
	}
	spec := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Synthetic singleton workflows","version":"test"},"servers":[{"url":"https://foreign.invalid"}],"paths":{%q:{"post":%s,"put":%s},%q:{"delete":{"parameters":[{"name":"signature","in":"path","required":true,"schema":{"type":"string"}},{"name":"collection","in":"query","schema":{"type":"string"}}],"responses":{"200":{"description":"OK"}}}},%q:{"get":{"parameters":[{"name":"collection","in":"query","required":true,"schema":{"type":"string"}},{"name":"defaultIfUndefined","in":"query","schema":{"type":"boolean"}}],"responses":{"200":{"description":"OK"},"404":{"description":"Absent"}}}}}}`, base, body(`["collection","description"]`), body(`["collection","signature","description"]`), base+"/{signature}", read)
	var mu sync.Mutex
	exists, signature, description, writes := false, "", "", 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		if r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("selected-target credential changed")
		}
		if r.Method == "GET" && r.URL.Path == "/proxy"+read {
			if r.URL.RawQuery != "collection=core&defaultIfUndefined=false" {
				t.Error("singleton read substituted defaults or changed collection")
			}
			if !exists {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "test/settings", "collection": "core", "name": "server-alias", "signature": signature, "description": description, "config": map[string]any{"exact": json.Number("9007199254740993")}})
			return
		}
		writes++
		switch r.Method {
		case "POST", "PUT":
			var input []map[string]string
			if r.URL.Path != "/proxy"+base || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&input) != nil || len(input) != 1 {
				t.Error("invalid singleton mutation")
				w.WriteHeader(400)
				return
			}
			if _, ok := input[0]["name"]; ok {
				t.Error("singleton mutation invented a name")
			}
			if input[0]["collection"] != "core" || r.Method == "PUT" && input[0]["signature"] != signature || r.Method == "POST" && exists {
				t.Error("singleton precondition mismatch")
			}
			exists, description, signature = true, input[0]["description"], fmt.Sprintf("sig/+%%2F?&=日本-%d", writes)
		case "DELETE":
			if r.URL.EscapedPath() != "/proxy"+base+"/"+url.PathEscape(signature) || r.URL.RawQuery != "collection=core" {
				t.Error("signature path or delete query changed")
			}
			exists = false
		default:
			t.Error("unexpected operation")
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "changes": []any{map[string]any{"type": "test/settings", "collection": "core", "newSignature": signature}}})
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	count := func() int { mu.Lock(); defer mu.Unlock(); return writes }
	for index, action := range []string{"create", "update", "delete"} {
		args := []string{"resource", action, "test/settings", "--json"}
		if action != "delete" {
			args = append(args, "--body", `{"description":"private-config-value"}`)
		}
		if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
			t.Fatal(err)
		}
		preview := decodeResult(t, out)
		if preview.Outcome != "preview" || count() != index {
			t.Fatal("singleton preview mutated")
		}
		if action != "create" {
			bad := append(append([]string(nil), args...), "--if-signature", "stale", "--yes")
			if err := app.Run(context.Background(), bad); err == nil {
				t.Fatal("stale singleton review accepted")
			}
			if got := decodeResult(t, out); got.Error.Kind != "conflict" || count() != index {
				t.Fatal("stale review sent a mutation")
			}
			args = append(args, "--if-signature", preview.Data.(map[string]any)["beforeSignature"].(string))
		}
		if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
			t.Fatal(err)
		}
		output := out.String() + stderr.String()
		got := decodeResult(t, out)
		if !got.OK || got.Outcome != "completed" || got.Meta.Verification != "verified" || got.Data.(map[string]any)["singleton"] != true || count() != index+1 {
			t.Fatalf("singleton did not complete once: %+v", got)
		}
		if strings.Contains(output, "private-config-value") || strings.Contains(output, "private-token") {
			t.Fatal("workflow output disclosed input or credentials")
		}
	}
	for _, args := range [][]string{
		{"resource", "get", "test/settings", "invented-name"},
		{"resource", "get", "test/named-without-singleton"},
		{"resource", "update", "test/settings", "--body", `{"description":"new"}`, "--yes"},
	} {
		if err := app.Run(context.Background(), append(args, "--json")); err == nil {
			t.Fatal("invalid singleton mode or unreviewed write accepted")
		}
		if got := decodeResult(t, out); got.Error.Code != 2 || count() != 3 {
			t.Fatal("invalid input reached a mutation")
		}
	}
}
