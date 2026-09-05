package execute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestHeaderInputPreparedValuesMatchWire(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Test") != "\u00a0literal\u00a0" || !reflect.DeepEqual(r.Header.Values("X-Order"), []string{"one", "two"}) || r.Header.Get("Content-Type") != "text/plain" || len(r.Header.Values("X-None")) != 0 {
			t.Error("header normalization or prepared input changed before transmission")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	engine := Engine{HTTP: srv.Client()}
	headers := http.Header{"x-test": {" \t\u00a0literal\u00a0\t "}, "X-Order": {"one"}, "x-order": {"two"}, "X-None": nil}
	input := Request{Method: "POST", Path: "/headers", Yes: true, Headers: headers, ContentType: " \ttext/plain\t "}
	prepared, err := engine.Prepare(context.Background(), target, "private-token", input)
	if err != nil || calls.Load() != 0 {
		t.Fatalf("preparation failed or dispatched: %v", err)
	}
	if !reflect.DeepEqual(prepared.preview.HeaderKeys, []string{"X-Order", "X-Test"}) || prepared.preview.ContentType != "text/plain" || headers["x-test"][0] != " \t\u00a0literal\u00a0\t " {
		t.Fatal("preview differs from effective headers or preparation changed caller input")
	}
	headers["x-test"][0], headers["X-Order"][0] = "changed", "changed"
	if got := engine.Execute(context.Background(), prepared, "private-token"); !got.OK || calls.Load() != 1 {
		t.Fatal("prepared header execution failed")
	}
}

func TestHeaderInputTypedRefusesBeforeDiscovery(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	engine := Engine{HTTP: srv.Client(), Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: srv.Client()}}
	for _, headers := range []http.Header{
		{"X-Test": {"invalid\x00"}}, {"X-[]": {"value"}}, {"X-Test": {"\vvalue"}},
		{"X-Ignition-API-Token": {"override"}}, {"content-type": {"text/plain"}}, {"Host": {"foreign.test"}},
	} {
		_, err := engine.Prepare(context.Background(), target, "private-token", Request{Operation: "POST /headers", DryRun: true, Headers: headers})
		if err == nil || igwerr.ExitCode(err) != 2 || calls.Load() != 0 {
			t.Fatal("invalid typed headers reached discovery or escaped usage validation")
		}
	}
}
