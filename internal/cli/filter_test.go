package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func filterSpec(t *testing.T) []byte {
	t.Helper()
	const operation = `{"parameters":[
 {"name":"filter","in":"query","style":"form","explode":true,"schema":{"type":"object","propertyNames":{"pattern":"^[a-z]+\\[(eq|gt)\\]$"},"additionalProperties":{"type":"string"}}},
 {"name":"limit","in":"query","schema":{"type":"integer","minimum":1}},
 {"name":"offset","in":"query","schema":{"type":"integer","minimum":0}}
 ],"responses":{"200":{"description":"OK"}}}`
	var op any
	if err := json.Unmarshal([]byte(operation), &op); err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]any)
	for _, path := range []string{"/data/api/v1/resources/list/ignition/schedule", "/data/api/v1/projects/list", "/data/api/v1/logs"} {
		paths[path] = map[string]any{"get": op}
	}
	b, err := json.Marshal(map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "Synthetic filter API", "version": "test"}, "paths": paths})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestListFiltersPreserveWireValuesAndShareValidation(t *testing.T) {
	for _, args := range [][]string{{"resource", "list", "ignition/schedule"}, {"project", "list"}, {"logs", "list"}} {
		t.Run(args[0], func(t *testing.T) {
			spec := filterSpec(t)
			var calls atomic.Int32
			var received atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/openapi.json") {
					_, _ = w.Write(spec)
					return
				}
				calls.Add(1)
				if r.Method != "GET" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("request method or managed authentication changed")
				}
				received.Store(r.URL.Query())
				_, _ = w.Write([]byte(`{"items":[]}`))
			}))
			defer srv.Close()
			app, out, _ := testApp(t, srv)
			value := "true+text & % # / = 日本"
			command := append(append([]string(nil), args...), "--filter", "name[eq]="+value, "--filter", "count[gt]=9007199254740993", "--json")
			if err := app.Run(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			got := decodeResult(t, out)
			want := url.Values{"limit": {"50"}, "offset": {"0"}, "name[eq]": {value}, "count[gt]": {"9007199254740993"}}
			if !got.OK || calls.Load() != 1 || !reflect.DeepEqual(received.Load(), want) {
				t.Fatalf("outgoing filter data changed: %+v", received.Load())
			}
			operation := map[string]string{"resource": "GET /data/api/v1/resources/list/ignition/schedule", "project": "GET /data/api/v1/projects/list", "logs": "GET /data/api/v1/logs"}[args[0]]
			if err := app.Run(context.Background(), []string{"api", "request", operation, "--query", "name[eq]=" + value, "--dry-run", "--json"}); err != nil {
				t.Fatal(err)
			}
			got = decodeResult(t, out)
			if !got.OK || got.Outcome != "preview" || calls.Load() != 1 {
				t.Fatal("generic request did not share filter validation or sent its preview")
			}
			bad := append(append([]string(nil), args...), "--filter", "name[unknown]=value", "--json")
			err := app.Run(context.Background(), bad)
			got = decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || got.OK || got.Error.Kind != "validation" || calls.Load() != 1 {
				t.Fatal("invalid filter reached an operation or bypassed catalog validation")
			}
		})
	}
}

func TestListFilterUsagePrecedesConfiguration(t *testing.T) {
	for _, args := range [][]string{{"resource", "list", "ignition/schedule"}, {"project", "list"}, {"logs", "list"}} {
		for _, filters := range [][]string{{"name=value"}, {"name[]=value"}, {"name[eq]"}, {"name[eq][gt]=value"}, {"name[eq]=one", "name[eq]=two"}} {
			app, out, _ := testApp(t, nil)
			app.ReadConfig = func() (config.File, error) {
				t.Fatal("invalid filter loaded Gateway configuration")
				return config.File{}, nil
			}
			command := append([]string(nil), args...)
			for _, filter := range filters {
				command = append(command, "--filter", filter)
			}
			err := app.Run(context.Background(), append(command, "--json"))
			got := decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || got.OK {
				t.Fatal("malformed or duplicate filter accepted")
			}
		}
	}
}
