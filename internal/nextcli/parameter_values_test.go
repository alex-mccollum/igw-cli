package nextcli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestAPIQueryValuesKeepTheirWireRepresentation(t *testing.T) {
	const spec = `{"openapi":"3.1.0","info":{"title":"Synthetic typed query","version":"test"},"paths":{"/query":{"get":{"parameters":[
{"name":"enabled","in":"query","required":true,"schema":{"type":"boolean","const":false}},
{"name":"amount","in":"query","required":true,"schema":{"type":"number","const":9007199254740993}},
{"name":"ids","in":"query","required":true,"schema":{"type":"array","minItems":2,"maxItems":2,"uniqueItems":true,"items":{"type":"string","minLength":2}}},
{"name":"text","in":"query","schema":{"type":"string","enum":[" text + & % # / = 日本 "]}}
],"responses":{"200":{"description":"OK"}}}}}}`
	var requests atomic.Int32
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = w.Write([]byte(spec))
			return
		}
		requests.Add(1)
		received.Store(r.URL.Query())
		if r.Method != "GET" || r.URL.Path != "/proxy/query" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("query request changed its method, path, or managed credentials")
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	valid := []string{"api", "request", "GET /query", "--query", "enabled=false", "--query", "amount=9007199254740993", "--query", "ids=red,blue", "--query", "ids=next", "--query", "text= text + & % # / = 日本 ", "--json"}
	if err := app.Run(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	want := url.Values{"enabled": {"false"}, "amount": {"9007199254740993"}, "ids": {"red,blue", "next"}, "text": {" text + & % # / = 日本 "}}
	if !got.OK || requests.Load() != 1 || !reflect.DeepEqual(received.Load(), want) {
		t.Fatal("typed query validation changed the outgoing values")
	}
	if err := app.Run(context.Background(), append(append([]string(nil), valid...), "--dry-run")); err != nil {
		t.Fatal(err)
	}
	got = decodeResult(t, out)
	if !got.OK || got.Outcome != "preview" || requests.Load() != 1 {
		t.Fatal("typed query preview sent an operation")
	}
	for _, replacement := range []struct {
		at    int
		value string
	}{{4, "enabled=true"}, {6, "amount=9007199254740992"}, {8, "ids=next"}, {10, "ids=a"}} {
		args := append([]string(nil), valid...)
		args[replacement.at] = replacement.value
		err := app.Run(context.Background(), args)
		got = decodeResult(t, out)
		if igwerr.ExitCode(err) != 2 || got.OK || got.Error.Kind != "validation" || requests.Load() != 1 {
			t.Fatal("invalid typed query reached the operation")
		}
	}
	err := app.Run(context.Background(), append(append([]string(nil), valid...), "--query", "amount=9007199254740993"))
	got = decodeResult(t, out)
	if igwerr.ExitCode(err) != 2 || got.OK || got.Error.Kind != "validation" || requests.Load() != 1 {
		t.Fatal("repeated scalar parameter reached the operation")
	}
}
