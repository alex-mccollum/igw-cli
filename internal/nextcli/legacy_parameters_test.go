package nextcli

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

func TestLegacyUndocumentedInputCannotExecuteOrPreview(t *testing.T) {
	t.Parallel()
	const spec = `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1.0.0","license":{"url":"/res/sys/license.html","name":"Inductive Automation EULA"}},"paths":{"/data/api/v1/scripts/cancel-script/{id}":{"delete":{"parameters":[{"name":"id","in":"path","description":"n/a","required":true,"deprecated":false,"style":"simple","explode":false,"allowReserved":false}],"responses":{"200":{"description":"OK"}}}}}}`
	var operations atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		operations.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	for _, mode := range []string{"--dry-run", "--yes"} {
		app, out, stderr := testApp(t, srv)
		err := app.Run(context.Background(), []string{"api", "request", "DELETE /data/api/v1/scripts/cancel-script/{id}", "--path-param", "id=private-input", mode, "--json"})
		if igwerr.ExitCode(err) != 2 || strings.Contains(out.String(), "private-input") || stderr.Len() != 0 {
			t.Fatalf("undocumented schema failure contract: %v", err)
		}
		r := decodeResult(t, out)
		if r.OK || r.Error == nil || r.Error.Kind != "catalog_schema" {
			t.Fatalf("undocumented schema accepted: %+v", r)
		}
	}
	if operations.Load() != 0 {
		t.Fatal("undocumented operation reached the Gateway")
	}
	// The explicit raw escape hatch retains its confirmation requirement.
	for _, confirmed := range []bool{false, true} {
		app, out, _ := testApp(t, srv)
		args := []string{"api", "raw", "--method", "DELETE", "--path", "/data/api/v1/scripts/cancel-script/known", "--json"}
		if confirmed {
			args = append(args, "--yes")
		}
		err := app.Run(context.Background(), args)
		if (err == nil) != confirmed || decodeResult(t, out).OK != confirmed {
			t.Fatalf("raw confirmation contract changed: %v", err)
		}
	}
	if operations.Load() != 1 {
		t.Fatal("raw request did not execute exactly once after confirmation")
	}
}

func TestLegacySelectedPathRequiresEveryPlaceholder(t *testing.T) {
	t.Parallel()
	const spec = `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1.0.0","license":{"url":"/res/sys/license.html","name":"Inductive Automation EULA"}},"paths":{"/data/api/v1/entity/section/{section}":{"get":{"parameters":[{"name":"section","in":"path","required":false,"deprecated":false,"style":"simple","explode":false,"allowReserved":false,"schema":{"type":"string"}}],"responses":{"200":{"description":"OK"}}}}}}`
	var operations atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		operations.Add(1)
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	err := app.Run(context.Background(), []string{"api", "request", "GET /data/api/v1/entity/section/{section}", "--json"})
	if igwerr.ExitCode(err) != 2 || decodeResult(t, out).OK || operations.Load() != 0 {
		t.Fatalf("selected template sent without its placeholder: %v", err)
	}
}
