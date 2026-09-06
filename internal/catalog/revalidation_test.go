package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRevalidationTransfersOnlyUnchangedCatalog(t *testing.T) {
	for _, tc := range []struct {
		name, etag, modified, sourceKind, body string
		status                                 int
		reuse                                  bool
	}{
		{name: "etag 304", etag: `"first"`, sourceKind: "gateway", status: 304, reuse: true},
		{name: "last modified 304", modified: "Fri, 04 Sep 2026 00:00:00 GMT", sourceKind: "gateway", status: 304, reuse: true},
		{name: "identical response", sourceKind: "gateway", status: 200, body: testSpec, reuse: true},
		{name: "import becomes verified", sourceKind: "import", status: 200, body: testSpec, reuse: true},
		{name: "documentation changes", sourceKind: "gateway", status: 200, body: strings.Replace(testSpec, `"description":"OK"`, `"description":"Updated documentation"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/proxy/openapi.json" || r.Header.Get("If-None-Match") != tc.etag || r.Header.Get("If-Modified-Since") != tc.modified {
					t.Error("revalidation did not use the selected target and its validators")
				}
				w.Header().Set("ETag", `"response"`)
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer srv.Close()
			target, _ := NewTarget("test", srv.URL+"/proxy")
			parsed, err := Parse([]byte(testSpec))
			if err != nil {
				t.Fatal(err)
			}
			before := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
			now := before.Add(time.Hour)
			cached := &Snapshot{Catalog: parsed, Metadata: Metadata{
				Version: SnapshotVersion, Target: target, SourceKind: tc.sourceKind,
				Source: target.URL + "/openapi.json", FetchedAt: before, VerifiedAt: before,
				ETag: tc.etag, LastModified: tc.modified,
			}}
			defer cached.Close()
			svc := Service{Store: Store{Dir: t.TempDir()}, HTTP: srv.Client(), Now: func() time.Time { return now }}
			fresh, err := svc.fetch(context.Background(), target, "secret", cached)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			if tc.reuse != (fresh.Catalog == parsed) || tc.reuse != (cached.Catalog == nil) {
				t.Fatal("unchanged catalog ownership was not transferred after publication")
			}
			if calls.Load() != 1 || fresh.Stale || len(fresh.Warnings) != 0 || fresh.Metadata.VerifiedAt != now || fresh.Metadata.SourceKind != "gateway" || fresh.Metadata.Source != target.URL+"/openapi.json" {
				t.Fatal("revalidation lost fresh target evidence")
			}
			fetched := now
			etag := `"response"`
			if tc.status == 304 {
				fetched, etag = before, tc.etag
			}
			if fresh.Metadata.FetchedAt != fetched || fresh.Metadata.ETag != etag {
				t.Fatal("fetch provenance changed incorrectly")
			}
			if fresh.Catalog.ContractHash() != parsed.ContractHash() {
				t.Fatal("fixture unexpectedly changed its contract")
			}
			if !tc.reuse && (fresh.Catalog.RawHash() == parsed.RawHash() || fresh.Catalog.DocumentHash() == parsed.DocumentHash()) {
				t.Fatal("equal contract identity hid document changes")
			}
			if tc.reuse {
				cached.Close() // Acquire closes the consumed snapshot before returning.
			}
			assertRevalidatedCatalogUsable(t, fresh.Catalog)
			saved, err := svc.Store.Load(target)
			if err != nil {
				t.Fatal(err)
			}
			defer saved.Close()
			if saved.Metadata.VerifiedAt != now || saved.Catalog.RawHash() != fresh.Catalog.RawHash() {
				t.Fatal("new receipt did not preserve verified document identity")
			}
		})
	}
}

func TestRevalidationPublicationFailureRetainsCatalog(t *testing.T) {
	for _, status := range []int{200, 304} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(testSpec))
				}
			}))
			defer srv.Close()
			target, _ := NewTarget("test", srv.URL)
			c := testCatalog(t)
			before := time.Now().UTC().Add(-time.Hour)
			cached := &Snapshot{Catalog: c, Metadata: Metadata{Version: SnapshotVersion, Target: target,
				SourceKind: "gateway", Source: target.URL + "/openapi.json", ETag: `"first"`, FetchedAt: before, VerifiedAt: before}}
			blocked := filepath.Join(t.TempDir(), "not-a-directory")
			if err := os.WriteFile(blocked, []byte("existing file"), 0600); err != nil {
				t.Fatal(err)
			}
			svc := Service{Store: Store{Dir: blocked}, HTTP: srv.Client()}
			if fresh, err := svc.fetch(context.Background(), target, "secret", cached); err == nil || fresh != nil {
				fresh.Close()
				t.Fatal("failed publication returned a fresh snapshot")
			}
			if cached.Catalog != c || cached.Metadata.VerifiedAt != before {
				t.Fatal("publication failure consumed or refreshed the fallback")
			}
			assertRevalidatedCatalogUsable(t, c)
		})
	}
}

func TestRevalidationRefusesUnsolicitedNotModified(t *testing.T) {
	for _, source := range []string{"gateway", "import"} {
		t.Run(source, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
					t.Error("conditional request used an unverified imported validator")
				}
				w.WriteHeader(http.StatusNotModified)
			}))
			defer srv.Close()
			target, _ := NewTarget("test", srv.URL)
			before := time.Now().UTC().Add(-time.Hour)
			cached := &Snapshot{Catalog: testCatalog(t), Metadata: Metadata{Version: SnapshotVersion, Target: target,
				SourceKind: source, Source: target.URL + "/openapi.json", FetchedAt: before, VerifiedAt: before}}
			if source == "import" {
				cached.Metadata.ETag = `"unverified"`
				cached.Metadata.LastModified = "Fri, 04 Sep 2026 00:00:00 GMT"
			}
			svc := Service{Store: Store{Dir: t.TempDir()}, HTTP: srv.Client()}
			if fresh, err := svc.fetch(context.Background(), target, "secret", cached); err == nil || fresh != nil {
				fresh.Close()
				t.Fatal("unsolicited 304 verified a catalog")
			}
			if cached.Catalog == nil || cached.Metadata.VerifiedAt != before {
				t.Fatal("unsolicited 304 changed fallback ownership or verification")
			}
			assertRevalidatedCatalogUsable(t, cached.Catalog)
			if saved, err := svc.Store.Load(target); err == nil {
				saved.Close()
				t.Fatal("unsolicited 304 published a new receipt")
			}
		})
	}
}

func assertRevalidatedCatalogUsable(t *testing.T, c *Catalog) {
	t.Helper()
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"enabled":true,"count":9007199254740993}`, true},
		{`{"enabled":true,"count":9007199254740992}`, false},
	} {
		req := httptest.NewRequest("PUT", "http://gateway.test/items/test", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("PUT /items/{name}", req)
		if err != nil || (len(issues) == 0) != tc.valid {
			t.Fatalf("revalidated model lost its referenced exact-number constraint: %v %v", issues, err)
		}
	}
}
