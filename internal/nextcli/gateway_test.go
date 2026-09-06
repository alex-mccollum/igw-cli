package nextcli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/operations"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

func restartSpec(missing string) string {
	paths := map[string]any{}
	for _, key := range workflow.GatewayRestart().RequiredOperations {
		method, path, _ := strings.Cut(key, " ")
		if path == missing {
			continue
		}
		op := map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}}
		if method == "POST" {
			op["parameters"] = []any{map[string]any{"name": "confirm", "in": "query", "schema": map[string]any{"type": "boolean"}}}
		}
		paths[path] = map[string]any{strings.ToLower(method): op}
	}
	b, _ := json.Marshal(map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Synthetic restart API", "version": "test"}, "paths": paths})
	return string(b)
}

func TestGatewayRestartThroughSharedExecution(t *testing.T) {
	for _, scenario := range []string{"confirmed", "preview", "disconnect", "auth after restart", "redirect", "invalid baseline", "missing capability"} {
		t.Run(scenario, func(t *testing.T) {
			var catalogs, reads, writes atomic.Int32
			spec := restartSpec("")
			if scenario == "missing capability" {
				spec = restartSpec("/data/api/v1/redundancy")
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("credential binding lost")
				}
				if r.URL.Path == "/proxy/openapi.json" {
					catalogs.Add(1)
					_, _ = io.WriteString(w, spec)
					return
				}
				if !strings.HasPrefix(r.URL.Path, "/proxy/data/api/v1/") {
					t.Error("unexpected route or followed redirect")
					w.WriteHeader(500)
					return
				}
				if r.Method == "POST" {
					writes.Add(1)
					if r.URL.Path != "/proxy/data/api/v1/restart-tasks/restart" || r.URL.RawQuery != "confirm=true" || r.ContentLength != 0 || reads.Load() != 4 || catalogs.Load() != 2 {
						t.Error("restart dispatched without its exact baseline and fresh catalog")
					}
					if scenario == "disconnect" {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
						return
					}
					if scenario == "redirect" {
						w.Header().Set("Location", "/must-not-follow")
						w.WriteHeader(307)
						return
					}
					w.WriteHeader(200)
					return
				}
				reads.Add(1)
				if scenario == "auth after restart" && writes.Load() > 0 {
					w.WriteHeader(403)
					return
				}
				switch strings.TrimPrefix(r.URL.Path, "/proxy") {
				case "/data/api/v1/redundancy":
					if scenario == "invalid baseline" {
						_, _ = io.WriteString(w, `{"localId":null}`)
						return
					}
					_, _ = io.WriteString(w, `{"localId":"node-a"}`)
				case "/data/api/v1/overview":
					if writes.Load() == 0 {
						_, _ = io.WriteString(w, `{"processId":5,"uptime":200}`)
					} else {
						_, _ = io.WriteString(w, `{"processId":6,"uptime":2}`)
					}
				case "/data/api/v1/restart-tasks/pending":
					_, _ = io.WriteString(w, `{"pending":[]}`)
				default:
					t.Error("unknown restart read")
				}
			}))
			defer srv.Close()
			app, buf, _ := testApp(t, srv)
			// Seed the cache, then require write invocations to revalidate once.
			if err := app.Run(context.Background(), []string{"spec", "sync", "--json"}); err != nil {
				t.Fatal(err)
			}
			buf.Reset()
			confirmation := "--yes"
			if scenario == "preview" {
				confirmation = "--dry-run"
			}
			err := app.Run(context.Background(), []string{"gateway", "restart", confirmation, "--timeout", "3s", "--interval", "100ms", "--json"})
			out := decodeResult(t, buf)
			var evidence operations.RestartEvidence
			b, _ := json.Marshal(out.Data)
			_ = json.Unmarshal(b, &evidence)
			switch scenario {
			case "confirmed", "disconnect":
				if err != nil || !out.OK || out.Outcome != "completed" || out.Meta.Verification != "restart_observed" || writes.Load() != 1 || reads.Load() != 8 || evidence.Before.ProcessID != 5 || evidence.Last.ProcessID != 6 || evidence.Acknowledged != (scenario == "confirmed") {
					t.Fatalf("restart failed: %+v %v", out, err)
				}
			case "preview":
				if err != nil || out.Outcome != "preview" || writes.Load() != 0 || reads.Load() != 4 || catalogs.Load() != 1 || evidence.Request == nil || !evidence.Request.Mutating || len(evidence.Request.QueryKeys) != 1 || evidence.Request.QueryKeys[0] != "confirm" {
					t.Fatalf("bad preview: %+v", out)
				}
			case "auth after restart":
				if err == nil || out.Error.Code != 6 || out.Outcome != "uncertain" || writes.Load() != 1 || reads.Load() != 5 {
					t.Fatalf("auth contract: %+v", out)
				}
			case "redirect":
				if err == nil || out.Error.Code != 7 || out.Outcome != "failed" || writes.Load() != 1 || reads.Load() != 4 {
					t.Fatalf("redirect replayed: %+v", out)
				}
			case "invalid baseline":
				if err == nil || out.Error.Kind != "response" || writes.Load() != 0 || reads.Load() != 1 {
					t.Fatalf("invalid baseline authorized restart: %+v", out)
				}
			case "missing capability":
				if err == nil || out.Error.Kind != "capability" || out.Error.Code != 2 || writes.Load() != 0 || reads.Load() != 0 {
					t.Fatalf("missing prerequisite dispatched: %+v", out)
				}
			}
			if out.Meta.Catalog == nil || out.Meta.Target == nil || out.Meta.Target.URL != srv.URL+"/proxy" {
				t.Fatal("restart evidence lost catalog/target provenance")
			}
		})
	}
}

func TestGatewayRestartUsageBeforeConfiguration(t *testing.T) {
	for _, args := range [][]string{{"restart"}, {"restart", "--yes", "--dry-run"}, {"restart", "--yes", "--interval", "0s"}, {"restart", "--dry-run", "--interval", "2m"}, {"restart", "--dry-run", "--offline"}, {"restart-tasks", "--offline"}} {
		app, buf, _ := testApp(t, nil)
		app.ReadConfig = func() (config.File, error) {
			t.Fatal("invalid restart loaded configuration")
			return config.File{}, nil
		}
		err := app.Run(context.Background(), append(append([]string{"gateway"}, args...), "--json"))
		out := decodeResult(t, buf)
		if err == nil || out.Error.Code != 2 {
			t.Fatalf("invalid usage accepted: %v", args)
		}
	}
}

func TestGatewayRestartSchemaWithoutConnectivity(t *testing.T) {
	app, buf, _ := testApp(t, nil)
	app.ReadConfig = func() (config.File, error) { t.Fatal("schema read configuration"); return config.File{}, nil }
	if err := app.Run(context.Background(), []string{"gateway", "restart", "--help", "--json"}); err != nil {
		t.Fatal(err)
	}
	var info commandInfo
	out := decodeResult(t, buf)
	b, _ := json.Marshal(out.Data)
	if json.Unmarshal(b, &info) != nil {
		t.Fatal("invalid schema")
	}
	flags := map[string]string{}
	for _, flag := range info.Flags {
		flags[flag.Name] = flag.Type
	}
	if flags["yes"] != "bool" || flags["dry-run"] != "bool" || flags["interval"] != "duration" || flags["timeout"] != "duration" {
		t.Fatalf("restart flags absent: %+v", info)
	}
}
