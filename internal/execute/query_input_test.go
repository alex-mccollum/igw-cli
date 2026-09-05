package execute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestQueryInputPreparedNamesMatchWire(t *testing.T) {
	want := url.Values{" name ": {"one=value", "two"}, "key=part": {"private-value"}, "\u00a0name\u00a0": {""}, "+&%#[]日本": {" + & % # / = 日本 "}}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !reflect.DeepEqual(r.URL.Query(), want) || r.URL.Path != "/proxy/query" {
			t.Error("prepared query names or values changed on the wire")
		}
		if !reflect.DeepEqual(r.URL.Query()["key=part"], want["key=part"]) {
			t.Error("an equals sign in a typed query name was reinterpreted")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL+"/proxy")
	engine := Engine{HTTP: srv.Client()}
	query := make(url.Values)
	var names []string
	for key, values := range want {
		query[key] = append([]string(nil), values...)
		names = append(names, key)
	}
	sort.Strings(names)
	query["omitted"], query["also-omitted"] = nil, []string{}
	prepared, err := engine.Prepare(context.Background(), target, "private-token", Request{Method: "POST", Path: "/query", Query: query, Yes: true})
	if err != nil || calls.Load() != 0 {
		t.Fatalf("preparation failed or dispatched: %v", err)
	}
	if !reflect.DeepEqual(prepared.preview.QueryKeys, names) || len(query) != len(want)+2 {
		t.Error("preview includes unsent query keys or changed caller input")
	}
	query[" name "][0] = "changed"
	query["extra"] = []string{"changed"}
	if got := engine.Execute(context.Background(), prepared, "private-token"); !got.OK || calls.Load() != 1 {
		t.Fatal("prepared query execution failed")
	}
}

func TestQueryInputTypedRefusesBeforeDiscovery(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	engine := Engine{HTTP: srv.Client(), Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: srv.Client()}}
	for _, operation := range []string{"", "POST /query"} {
		_, err := engine.Prepare(context.Background(), target, "private-token", Request{Operation: operation, Method: "POST", Path: "/query", DryRun: true, Query: url.Values{"": {"private-query-value"}}})
		if err == nil || igwerr.ExitCode(err) != 2 || calls.Load() != 0 {
			t.Fatal("empty typed query name reached discovery or passed preview")
		}
	}
}
