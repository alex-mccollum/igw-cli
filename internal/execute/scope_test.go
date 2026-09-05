package execute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

const scopeSpec = `{"openapi":"3.1.0","info":{"title":"Scope test","version":"test"},"paths":{"/state":{"get":{"responses":{"200":{"description":"OK"}}},"put":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["enabled"],"properties":{"enabled":{"type":"boolean"}}}}}},"responses":{"200":{"description":"OK"}}}}}}`

func TestScopeSharesFreshCatalogAcrossWorkflow(t *testing.T) {
	var specs, reads, writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("credential missing")
		}
		if r.URL.Path == "/openapi.json" {
			specs.Add(1)
			_, _ = io.WriteString(w, scopeSpec)
			return
		}
		if r.Method == "GET" {
			reads.Add(1)
		} else {
			writes.Add(1)
		}
		_, _ = io.WriteString(w, `{"enabled":true}`)
	}))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	e := Engine{Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: srv.Client()}, HTTP: srv.Client()}
	for n := 0; n < 2; n++ {
		scope, err := e.Open(context.Background(), target, "private-token", catalog.Policy{ForWrite: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, req := range []Request{
			{Operation: "GET /state"},
			{Operation: "PUT /state", Body: []byte(`{"enabled":true}`), DryRun: true},
			{Operation: "PUT /state", Body: []byte(`{"enabled":true}`), Yes: true},
			{Operation: "GET /state"},
		} {
			got := scope.Run(req)
			if !got.OK || got.Meta.Catalog == nil {
				t.Fatalf("step failed: %+v", got)
			}
		}
		if got := scope.Run(Request{Operation: "PUT /state", Body: []byte(`{"enabled":"secret"}`), Yes: true}); got.OK || got.Error.Kind != "validation" {
			t.Fatal("scope bypassed request validation")
		}
		scope.Close()
		scope.Close()
		if scope.Run(Request{Operation: "GET /state"}).OK {
			t.Fatal("closed scope sent request")
		}
	}
	if specs.Load() != 2 || reads.Load() != 4 || writes.Load() != 2 {
		t.Fatalf("unexpected requests: specs=%d reads=%d writes=%d", specs.Load(), reads.Load(), writes.Load())
	}
}

func TestReadScopeCannotEscalateOrBypassCatalog(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi.json" {
			calls.Add(1)
		}
		_, _ = io.WriteString(w, scopeSpec)
	}))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	e := Engine{Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: srv.Client()}, HTTP: srv.Client()}
	scope, err := e.Open(context.Background(), target, "private-token", catalog.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	for _, req := range []Request{
		{Operation: "PUT /state", Body: []byte(`{"enabled":true}`), Yes: true},
		{Method: "PUT", Path: "/state", Yes: true},
		{Operation: "GET /state", Pin: strings.Repeat("0", 64)},
		{Operation: "GET /state", AllowStale: true},
	} {
		if got := scope.Run(req); got.OK || got.Error.Code != 2 {
			t.Fatalf("unsafe step: %+v", got)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("rejected step reached Gateway")
	}
}

func TestScopeCloseCancelsInflightStep(t *testing.T) {
	entered := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi.json" {
			_, _ = io.WriteString(w, scopeSpec)
			return
		}
		close(entered)
		<-r.Context().Done()
	}))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	e := Engine{Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: srv.Client()}, HTTP: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	scope, err := e.Open(ctx, target, "private-token", catalog.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan bool, 1)
	go func() { finished <- scope.Run(Request{Operation: "GET /state"}).OK }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request never started")
	}
	scope.Close()
	select {
	case ok := <-finished:
		if ok {
			t.Fatal("canceled step succeeded")
		}
	case <-ctx.Done():
		t.Fatal("close did not cancel step")
	}
}
