package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
	"github.com/alex-mccollum/igw-cli/internal/resource"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func TestLogsHumanViewPreservesJSONAndFilters(t *testing.T) {
	const payload = `{"items":[{"timestamp":1788620400123,"level":"ERROR","loggerName":"Gateway","message":"Device failed\ninspect connection\u001b[2J","stack":["java.lang.Exception: refused","Caused by: offline",{"file":"Driver.java","line":12}],"mdc":{"request":"trace-1"},"futureField":"retained"}],"metadata":{"total":9,"matching":3,"limit":1,"offset":0},"futurePage":"also retained"}`
	const spec = `{"openapi":"3.1.0","info":{"title":"Synthetic log view","version":"test"},"paths":{"/data/api/v1/logs":{"get":{"parameters":[{"name":"limit","in":"query","schema":{"type":"integer"}},{"name":"offset","in":"query","schema":{"type":"integer"}},{"name":"minLevel","in":"query","schema":{"type":"string"}},{"name":"logger","in":"query","schema":{"type":"string"}},{"name":"search","in":"query","schema":{"type":"string"}},{"name":"startTime","in":"query","schema":{"type":"integer"}},{"name":"endTime","in":"query","schema":{"type":"integer"}}],"responses":{"200":{"description":"OK"}}}}}}`
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("log read mutated")
		}
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		requests.Add(1)
		q := r.URL.Query()
		if r.URL.Path != "/proxy/data/api/v1/logs" || q.Get("minLevel") != "WARN" || q.Get("startTime") != "1788616800123" || q.Get("endTime") != "1788620400123" || q.Get("logger") != "A&B" || q.Get("search") != "日本 + timeout" || q.Get("limit") != "1" || q.Get("offset") != "0" {
			t.Errorf("lost log selection: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	app.Now = func() time.Time { return time.UnixMilli(1788620400123) }
	args := []string{"logs", "list", "--min-level", "warn", "--since", "1h", "--until", "2026-09-05T08:00:00.123-07:00", "--logger", "A&B", "--search", "日本 + timeout", "--limit", "1"}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2026-09-05T15:00:00.123Z  ERROR  Gateway", "Device failed\n  inspect connection", `\x1b[2J`, "Caused by: offline", "Driver.java", "trace-1", "futureField", "futurePage", "3 matching", "--offset 1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("log view omitted %q: %s", want, out)
		}
	}
	if strings.Contains(out.String(), "\x1b") || stderr.Len() != 0 {
		t.Fatal("log view emitted terminal controls or warnings")
	}
	out.Reset()
	if err := app.Run(context.Background(), append(args, "--json")); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	var expected any
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Data, expected) || stderr.Len() != 0 || requests.Load() != 2 {
		t.Fatal("human view changed JSON data or dispatched extra requests")
	}
}

func TestLogsEmptyPageAndUnknownShapes(t *testing.T) {
	for _, tc := range []struct {
		name, payload, want string
		fallback            bool
	}{
		{"empty page", `{"items":[],"metadata":{"total":100,"matching":5,"limit":50,"offset":50}}`, "No log events on this page.", false},
		{"unknown row", `{"items":[{"message":"keep this unknown row","newField":12}]}`, "newField", true},
		{"null rows", `{"items":null,"reason":"keep this response"}`, "keep this response", true},
		{"unknown stack", `{"items":[{"timestamp":1,"level":"WARN","loggerName":"Gateway","message":"problem","stack":{"cause":"keep unusual stack"}}]}`, "keep unusual stack", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := humanLogs(&out, result.Success(json.RawMessage(tc.payload))); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) || strings.Contains(out.String(), "Next page:") {
				t.Fatalf("incorrect page: %s", &out)
			}
			if tc.fallback {
				var got, expected any
				if json.Unmarshal(out.Bytes(), &got) != nil || json.Unmarshal([]byte(tc.payload), &expected) != nil || !reflect.DeepEqual(got, expected) {
					t.Fatal("unknown shape lost original data")
				}
			}
		})
	}
}

func TestHumanResourceFailureRetainsVerificationEvidence(t *testing.T) {
	const spec = `{"openapi":"3.1.0","info":{"title":"Synthetic uncertain resource","version":"test"},"paths":{"/data/api/v1/resources/test/settings":{"post":{"requestBody":{"content":{"application/json":{"schema":{"type":"array","items":{"type":"object"}}}}},"responses":{"200":{"description":"OK"}}}},"/data/api/v1/resources/singleton/test/settings":{"get":{"parameters":[{"name":"collection","in":"query","schema":{"type":"string"}},{"name":"defaultIfUndefined","in":"query","schema":{"type":"boolean"}}],"responses":{"200":{"description":"OK"},"404":{"description":"Missing"}}}}}}`
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			writes.Add(1)
			_, _ = io.WriteString(w, `{"success":true,"changes":[{"type":"test/settings","collection":"core","newSignature":"observed"}]}`)
			return
		}
		if writes.Load() == 0 {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, `{"type":"test/settings","collection":"core","signature":"observed","description":"private-normalized"}`)
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	err := app.Run(context.Background(), []string{"resource", "create", "test/settings", "--body", `{"description":"private-requested"}`, "--yes"})
	if igwerr.ExitCode(err) != 7 || writes.Load() != 1 {
		t.Fatalf("wrong uncertainty or replay: %v", err)
	}
	for _, want := range []string{"Resource create: uncertain", "changed_unverified", "Signature matched: true", "Fields matched: false", "Mismatched fields: description"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("human failure hid %q: %s", want, out)
		}
	}
	if !strings.Contains(stderr.String(), "do not automatically retry") || strings.Contains(out.String()+stderr.String(), "private-") {
		t.Fatal("missing recovery guidance or leaked instance value")
	}
}

func TestHumanResourcePreviewAndOutputFailure(t *testing.T) {
	var out bytes.Buffer
	r := result.Success(resource.Evidence{Action: "update", Type: "test/settings", Name: "unsafe\x1b[2J", Collection: "core", BeforeSignature: "reviewed", ChangedFields: []string{"description"}, State: "not_changed"})
	r.Outcome = "preview"
	if err := human(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--if-signature") || !strings.Contains(out.String(), "reviewed") || strings.Contains(out.String(), "\x1b") {
		t.Fatal("preview lost review guidance or emitted terminal controls")
	}
	if err := human(failedHumanWriter{}, r); err == nil {
		t.Fatal("hidden output failure")
	}
}

type failedHumanWriter struct{}

func (failedHumanWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestTroubleshootingHTTPFailuresAreNotEmptyLogs(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var reads atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, `{"openapi":"3.1.0","info":{"title":"Synthetic failed logs","version":"test"},"paths":{"/data/api/v1/logs":{"get":{"parameters":[{"name":"limit","in":"query","schema":{"type":"integer"}},{"name":"offset","in":"query","schema":{"type":"integer"}}],"responses":{"200":{"description":"OK"}}}}}}`)
					return
				}
				reads.Add(1)
				w.WriteHeader(status)
			}))
			defer srv.Close()
			app, out, stderr := testApp(t, srv)
			err := app.Run(context.Background(), []string{"logs", "list"})
			if err == nil || out.Len() != 0 || !strings.Contains(stderr.String(), "Next:") || reads.Load() != 1 {
				t.Fatal("failed log request looked empty or omitted recovery guidance")
			}
		})
	}
}

func TestDoctorExplainsScopeWithoutChangingJSON(t *testing.T) {
	const payload = `{"name":"Test Gateway","ignitionVersion":"8.3.9","futureField":42}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("doctor mutated")
		}
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, fixtureSpec)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	if err := app.Run(context.Background(), []string{"gateway", "doctor"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Gateway information request succeeded") || !strings.Contains(out.String(), "module, device, or project problems") || !strings.Contains(out.String(), "futureField") {
		t.Fatal("doctor lost scope or details")
	}
	out.Reset()
	if err := app.Run(context.Background(), []string{"gateway", "doctor", "--json"}); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	if got.Data.(map[string]any)["futureField"] != json.Number("42") || stderr.Len() != 0 {
		t.Fatal("doctor changed JSON or emitted extra diagnostics")
	}
}

func TestWorkflowHelpExamplesParseWithoutGateway(t *testing.T) {
	i := &invocation{}
	count := 0
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, line := range strings.Split(cmd.Example, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			args, err := exampleWords(line)
			if err != nil || len(args) < 2 {
				t.Fatalf("invalid example: %s", line)
			}
			if err := validateExample(args[1:]); err != nil {
				t.Fatalf("example %s: %v", line, err)
			}
			count++
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(i.commands())
	if count < 12 {
		t.Fatal("workflow help examples missing")
	}
}
