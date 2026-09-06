package execute

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type batchServer struct {
	server  *httptest.Server
	mu      sync.Mutex
	specs   int
	methods []string
	next    func(http.ResponseWriter, *http.Request, int)
}

func newBatchServer(t *testing.T) (*batchServer, Engine, catalog.Target) {
	t.Helper()
	b := &batchServer{}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("missing managed credential")
		}
		b.mu.Lock()
		if r.URL.Path == "/openapi.json" {
			b.specs++
			b.mu.Unlock()
			_, _ = io.WriteString(w, scopeSpec)
			return
		}
		b.methods = append(b.methods, r.Method)
		n := len(b.methods)
		next := b.next
		b.mu.Unlock()
		if next != nil {
			next(w, r, n)
			return
		}
		_, _ = io.WriteString(w, `{"enabled":true,"count":9007199254740993}`)
	}))
	t.Cleanup(b.server.Close)
	target, _ := catalog.NewTarget("batch", b.server.URL)
	engine := Engine{Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: b.server.Client()}, HTTP: b.server.Client()}
	return b, engine, target
}

func batchItems() []BatchItem {
	return []BatchItem{
		{ID: "before", Request: Request{Operation: "GET /state"}},
		{ID: "change", Request: Request{Operation: "PUT /state", Body: []byte(`{"enabled":true}`)}},
		{ID: "after", Request: Request{Operation: "GET /state"}},
	}
}

func assertBatchReport(t *testing.T, got result.Result, outcome string, code int, want []string) BatchReport {
	t.Helper()
	report, ok := got.Data.(BatchReport)
	if !ok {
		t.Fatalf("missing batch report: %+v", got)
	}
	actual := make([]string, len(report.Items))
	for n, item := range report.Items {
		actual[n] = item.Result.Outcome
	}
	if got.Outcome != outcome || got.OK != (code == 0) || code != 0 && (got.Error == nil || got.Error.Code != code) || !reflect.DeepEqual(actual, want) {
		t.Fatalf("batch outcome=%s code=%+v items=%v, want %s/%d/%v", got.Outcome, got.Error, actual, outcome, code, want)
	}
	if report.Succeeded+report.Failed+report.NotRun != len(want) {
		t.Fatal("inconsistent item counts")
	}
	return report
}

func TestBatchSharesFreshCatalogAndPreservesResults(t *testing.T) {
	b, engine, target := newBatchServer(t)
	for n := 0; n < 2; n++ {
		got := engine.Batch(context.Background(), target, "private-token", batchItems(), BatchOptions{Yes: true})
		report := assertBatchReport(t, got, "accepted", 0, []string{"completed", "accepted", "completed"})
		if report.Succeeded != 3 || got.Meta.Catalog == nil || got.Meta.Verification != "not_performed" {
			t.Fatal("missing request evidence")
		}
		for j, item := range report.Items {
			if item.ID != batchItems()[j].ID || item.Result.Meta.Catalog.ContractSHA256 != got.Meta.Catalog.ContractSHA256 {
				t.Fatal("item order or catalog identity changed")
			}
		}
		encoded, _ := json.Marshal(got)
		if !strings.Contains(string(encoded), "9007199254740993") || strings.Contains(string(encoded), "private-token") {
			t.Fatal("precision lost or credential leaked")
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.specs != 2 || !reflect.DeepEqual(b.methods, []string{"GET", "PUT", "GET", "GET", "PUT", "GET"}) {
		t.Fatalf("unexpected requests: %d %v", b.specs, b.methods)
	}
}

func TestBatchPreviewAndConfirmationSendNoOperations(t *testing.T) {
	b, engine, target := newBatchServer(t)
	got := engine.Batch(context.Background(), target, "private-token", batchItems(), BatchOptions{})
	assertBatchReport(t, got, "failed", 2, []string{"not_run", "not_run", "not_run"})
	for _, offline := range []bool{false, true} {
		got = engine.Batch(context.Background(), target, "private-token", batchItems(), BatchOptions{DryRun: true, Policy: catalog.Policy{Offline: offline}})
		report := assertBatchReport(t, got, "preview", 0, []string{"preview", "preview", "preview"})
		if p, ok := report.Items[1].Result.Data.(Preview); !ok || !p.BodyPresent || !p.Mutating {
			t.Fatal("missing exact write preview")
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.methods) != 0 || b.specs != 1 {
		t.Fatalf("preview or confirmation dispatched: %d %v", b.specs, b.methods)
	}
}

func TestBatchRetainsResultsAfterValidationFailure(t *testing.T) {
	for _, keepGoing := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "continue"}[keepGoing], func(t *testing.T) {
			b, engine, target := newBatchServer(t)
			items := batchItems()
			items[1].Request.Body = []byte(`{"enabled":"private-invalid-body"}`)
			want := []string{"completed", "failed", "not_run"}
			if keepGoing {
				want[2] = "completed"
			}
			got := engine.Batch(context.Background(), target, "private-token", items, BatchOptions{Yes: true, ContinueOnError: keepGoing})
			report := assertBatchReport(t, got, "partial", 2, want)
			if report.Items[1].Result.Error.Kind != "validation" {
				t.Fatal("lost item validation error")
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), "private-invalid-body") {
				t.Fatal("failed body leaked")
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			for _, method := range b.methods {
				if method != "GET" {
					t.Fatal("invalid write dispatched")
				}
			}
		})
	}
}

func TestBatchStopsAfterTerminalFailure(t *testing.T) {
	for _, mode := range []string{"auth", "uncertain", "canceled", "large"} {
		t.Run(mode, func(t *testing.T) {
			b, engine, target := newBatchServer(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			b.mu.Lock()
			b.next = func(w http.ResponseWriter, r *http.Request, n int) {
				if n == 1 {
					_, _ = io.WriteString(w, `{"prior":true}`)
					return
				}
				switch mode {
				case "auth":
					w.WriteHeader(http.StatusForbidden)
				case "uncertain":
					_, _ = io.Copy(io.Discard, r.Body)
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					conn.Close()
				case "canceled":
					cancel()
					<-r.Context().Done()
				case "large":
					_, _ = io.WriteString(w, strings.Repeat("x", MaxBatchResponseBytes+1))
				}
			}
			b.mu.Unlock()
			items := batchItems()
			if mode != "uncertain" {
				items[1].Request = Request{Operation: "GET /state"}
			}
			keepGoing := mode != "large"
			got := engine.Batch(ctx, target, "private-token", items, BatchOptions{Yes: true, ContinueOnError: keepGoing})
			outcome, itemOutcome, code := "partial", "failed", 7
			if mode == "uncertain" {
				outcome, itemOutcome = "uncertain", "uncertain"
			}
			if mode == "auth" {
				code = 6
			}
			report := assertBatchReport(t, got, outcome, code, []string{"completed", itemOutcome, "not_run"})
			if report.Succeeded != 1 || report.Failed != 1 || report.NotRun != 1 {
				t.Fatal("prior outcomes lost")
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			if len(b.methods) != 2 {
				t.Fatalf("terminal failure replayed or continued: %v", b.methods)
			}
		})
	}
}

func TestBatchReadOnlyContinuationAndExitPrecedence(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "terminal"}[terminal], func(t *testing.T) {
			b, engine, target := newBatchServer(t)
			b.mu.Lock()
			b.next = func(w http.ResponseWriter, r *http.Request, n int) {
				if n == 2 {
					status := http.StatusBadRequest
					if terminal {
						status = http.StatusForbidden
					}
					w.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(w, `{"read":true}`)
			}
			b.mu.Unlock()
			items := []BatchItem{
				{ID: "invalid", Request: Request{Operation: "unknown-operation"}},
				{ID: "read", Request: Request{Operation: "GET /state"}},
				{ID: "rejected", Request: Request{Operation: "GET /state"}},
				{ID: "last", Request: Request{Operation: "GET /state"}},
			}
			// Read-only batches need no confirmation. Continuation preserves the
			// first ordinary error, but a later terminal auth error takes priority.
			got := engine.Batch(context.Background(), target, "private-token", items, BatchOptions{ContinueOnError: true})
			code, last := 2, "completed"
			if terminal {
				code, last = 6, "not_run"
			}
			assertBatchReport(t, got, "partial", code, []string{"failed", "completed", "failed", last})
		})
	}
}

func TestBatchBoundsRepeatedInputNames(t *testing.T) {
	for _, location := range []string{"query", "headers"} {
		t.Run(location, func(t *testing.T) {
			// A compact manifest can expand into a much larger request because
			// each array entry repeats its field name on the wire.
			key := strings.Repeat("x", 4096)
			values := make([]string, MaxBatchInputBytes/len(key)+1)
			item := BatchItem{ID: "read", Request: Request{Operation: "GET /state"}}
			if location == "query" {
				item.Request.Query = map[string][]string{key: values}
			} else {
				item.Request.Headers = map[string][]string{key: values}
			}
			if err := ValidateBatch([]BatchItem{item}); err == nil {
				t.Fatal("repeated names bypassed the expanded input limit")
			}
			if location == "query" {
				item.Request.Query = map[string][]string{"q": values}
			} else {
				item.Request.Headers = map[string][]string{"X": values}
			}
			if err := ValidateBatch([]BatchItem{item}); err != nil {
				t.Fatalf("bounded repetitions were refused: %v", err)
			}
		})
	}
}

func TestBatchRejectsInvalidStructureBeforeDiscovery(t *testing.T) {
	for _, name := range []string{"empty", "too-many", "duplicate-id", "invalid-id", "raw", "per-item-confirmation", "artifact", "response-limit", "input-limit"} {
		t.Run(name, func(t *testing.T) {
			b, engine, target := newBatchServer(t)
			items := batchItems()
			switch name {
			case "empty":
				items = nil
			case "too-many":
				items = make([]BatchItem, MaxBatchItems+1)
			case "duplicate-id":
				items[1].ID = items[0].ID
			case "invalid-id":
				items[1].ID = "bad\nname"
			case "raw":
				items[1].Request = Request{Method: "POST", Path: "/state"}
			case "per-item-confirmation":
				items[1].Request.Yes = true
			case "artifact":
				items[1].Request.Out = "private-path"
			case "response-limit":
				items[1].Request.MaxBodyBytes = MaxBatchResponseBytes + 1
			case "input-limit":
				items[1].Request.Body = make([]byte, MaxBatchInputBytes+1)
			}
			got := engine.Batch(context.Background(), target, "private-token", items, BatchOptions{Yes: true})
			if got.OK || got.Error.Code != 2 {
				t.Fatal("invalid batch accepted")
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.specs != 0 || len(b.methods) != 0 {
				t.Fatal("invalid manifest caused I/O")
			}
		})
	}
}
