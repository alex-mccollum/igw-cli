package testgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func moduleResponse(items any, total, offset int) map[string]any {
	return map[string]any{"items": items, "metadata": map[string]any{"total": total, "matching": total, "limit": modulePageSize, "offset": offset}}
}

func TestModuleInventoryPaginatesAndKeepsObservedStates(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("sortBy") != "asc(id)" {
			t.Error("inventory request lost bounds or sort")
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if strings.HasSuffix(r.URL.Path, "/quarantined") {
			_ = json.NewEncoder(w).Encode(moduleResponse([]any{map[string]any{"id": "z.quarantined", "version": "1.0", "reason": "private-detail"}}, 1, offset))
			return
		}
		items := []any{}
		for n := offset; n < min(offset+100, 101); n++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("module.%03d", n), "version": "8.3.9", "state": "FAULTED", "exception": map[string]string{"message": "private-detail"}, "onStartup": "LeaveAlone", "shouldUpgrade": false})
		}
		_ = json.NewEncoder(w).Encode(moduleResponse(items, 101, offset))
	}))
	defer srv.Close()
	s := &Session{URL: srv.URL}
	first, err := s.readModuleInventory(context.Background(), s.HTTPClient())
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || len(first.Modules) != 102 || first.Modules[0].State != "FAULTED" || first.Modules[0].Collection != "healthy" || first.Modules[101].Collection != "quarantined" || first.Modules[101].State != "" || first.Modules[0].ShouldUpgrade == nil {
		t.Fatalf("observed inventory lost: %+v", first)
	}
	second, err := s.readModuleInventory(context.Background(), s.HTTPClient())
	if err != nil || first.SHA256 != second.SHA256 || first.ObservedAt.Equal(second.ObservedAt) {
		t.Fatalf("unstable module identity: %v", err)
	}
	b, _ := json.Marshal(first)
	if strings.Contains(string(b), "private-detail") {
		t.Fatal("module diagnostic details retained")
	}
}

func TestModuleInventoryRejectsIncompleteAndAmbiguousPages(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing items", func(p map[string]any) { delete(p, "items") }},
		{"null items", func(p map[string]any) { p["items"] = nil }},
		{"no metadata", func(p map[string]any) { delete(p, "metadata") }},
		{"no offset", func(p map[string]any) { delete(p["metadata"].(map[string]any), "offset") }},
		{"wrong offset", func(p map[string]any) { p["metadata"].(map[string]any)["offset"] = 1 }},
		{"fractional total", func(p map[string]any) { p["metadata"].(map[string]any)["total"] = 1.5 }},
		{"oversized total", func(p map[string]any) { p["metadata"].(map[string]any)["total"] = maxModules + 1 }},
		{"filtered results", func(p map[string]any) { p["metadata"].(map[string]any)["matching"] = 0 }},
		{"missing page", func(p map[string]any) { p["items"] = []any{} }},
		{"missing version", func(p map[string]any) { p["items"] = []any{map[string]any{"id": "module"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := moduleResponse([]any{map[string]any{"id": "module", "version": "1"}}, 1, 0)
			tc.edit(p)
			b, _ := json.Marshal(p)
			if _, _, err := modulePage(b, "healthy", 0); err == nil {
				t.Fatal("incomplete module page accepted")
			}
		})
	}
	if _, _, err := modulePage([]byte(`{"items":[],"items":[],"metadata":{"total":0,"matching":0,"limit":100,"offset":0}}`), "healthy", 0); err == nil {
		t.Fatal("duplicate JSON key accepted")
	}
	for _, mode := range []string{"duplicate", "moving"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
				total := 2
				if mode == "moving" && offset > 0 {
					total = 3
				}
				_ = json.NewEncoder(w).Encode(moduleResponse([]any{map[string]any{"id": "same", "version": "1"}}, total, offset))
			}))
			defer srv.Close()
			s := &Session{URL: srv.URL}
			if _, err := s.readModuleInventory(context.Background(), s.HTTPClient()); err == nil {
				t.Fatal("moving inventory accepted")
			}
		})
	}
}

func TestModuleInventoryHonorsCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	s := &Session{URL: srv.URL}
	if _, err := s.readModuleInventory(ctx, s.HTTPClient()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestCaptureWaitsForBothRoutesAndModuleVersions(t *testing.T) {
	var reads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/StatusPing":
			fmt.Fprint(w, `{"state":"RUNNING"}`)
		case "/data/app/login":
			http.Redirect(w, r, "/idp/default/authn/login?token=initial", 302)
		case "/idp/default/authn/login", "/idp/default/oidc/auth":
			fmt.Fprint(w, "ok")
		case "/idp/default/authn/next-challenge":
			fmt.Fprint(w, `{"token":"challenge","complete":true}`)
		case "/idp/default/authn/submit-challenge/basic":
			fmt.Fprint(w, `{"token":"accepted","success":true}`)
		case "/data/api/v1/gateway-info":
			fmt.Fprint(w, `{"ignitionVersion":"8.3.9"}`)
		case "/openapi.json":
			fmt.Fprint(w, `{"openapi":"3.1.0","paths":{"/health":{}}}`)
		case "/data/api/v1/modules/healthy":
			version := "8.3.9"
			if reads.Add(1) == 1 {
				version = "8.3.8"
			}
			_ = json.NewEncoder(w).Encode(moduleResponse([]any{map[string]any{"id": "module", "version": version, "state": "RUNNING"}}, 1, 0))
		case "/data/api/v1/modules/quarantined":
			_ = json.NewEncoder(w).Encode(moduleResponse([]any{}, 0, 0))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s := &Session{URL: srv.URL, password: "private"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.WaitOpenAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 4 || s.ModuleInventory == nil || s.ModuleInventory.Modules[0].Version != "8.3.9" {
		t.Fatal("capture ignored a changing module inventory")
	}
}
