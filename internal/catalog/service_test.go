package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestFreshnessRequiresEveryWriteToRevalidate(t *testing.T) {
	t.Parallel()
	var gets atomic.Int32
	var unavailable atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)
		if r.URL.Path != "/proxy/openapi.json" || r.Method != "GET" {
			t.Errorf("wrong discovery request: %s %s", r.Method, r.URL.Path)
		}
		if unavailable.Load() {
			w.WriteHeader(503)
			return
		}
		if r.Header.Get("If-None-Match") == `"first"` {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"first"`)
		_, _ = w.Write([]byte(testSpec))
	}))
	defer srv.Close()
	now := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	svc := Service{Store: Store{Dir: t.TempDir()}, HTTP: srv.Client(), Now: func() time.Time { return now }}
	target, _ := NewTarget("dev", srv.URL+"/proxy")
	acquire := func(policy Policy) *Snapshot {
		t.Helper()
		snapshot, err := svc.Acquire(context.Background(), target, "secret", policy)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(snapshot.Close)
		return snapshot
	}
	first := acquire(Policy{})
	acquire(Policy{})
	if gets.Load() != 1 {
		t.Fatal("fresh read fetched again")
	}
	now = now.Add(time.Hour)
	write := acquire(Policy{ForWrite: true})
	if gets.Load() != 2 || write.Metadata.VerifiedAt != now || write.Metadata.FetchedAt != first.Metadata.FetchedAt {
		t.Fatal("write did not conditionally revalidate")
	}
	unavailable.Store(true)
	if snapshot, err := svc.Acquire(context.Background(), target, "secret", Policy{ForWrite: true}); err == nil {
		snapshot.Close()
		t.Fatal("write used cache without explicit override")
	}
	stale := acquire(Policy{ForWrite: true, AllowStale: true})
	if !stale.Stale || len(stale.Warnings) == 0 {
		t.Fatal("stale write lacked visible evidence")
	}
	now = now.Add(25 * time.Hour)
	if snapshot := acquire(Policy{}); !snapshot.Stale {
		t.Fatal("offline fallback looked fresh")
	}
	if snapshot, err := svc.Acquire(context.Background(), target, "secret", Policy{Refresh: true}); err == nil {
		snapshot.Close()
		t.Fatal("explicit sync silently fell back")
	}
}

func TestPinsAndTargetIsolation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(testSpec)) }))
	defer srv.Close()
	svc := Service{Store: Store{Dir: t.TempDir()}, HTTP: srv.Client()}
	target, _ := NewTarget("dev", srv.URL)
	first, err := svc.Acquire(context.Background(), target, "secret", Policy{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if snapshot, err := svc.Acquire(context.Background(), target, "secret", Policy{Offline: true, Pin: strings.Repeat("0", 64)}); igwerr.ExitCode(err) != 2 {
		snapshot.Close()
		t.Fatalf("pin mismatch was accepted: %v", err)
	}
	for _, other := range []Target{{Profile: "stage", URL: target.URL}, {Profile: "dev", URL: target.URL + "/proxy"}} {
		if snapshot, err := svc.Acquire(context.Background(), other, "secret", Policy{Offline: true}); err == nil {
			snapshot.Close()
			t.Fatal("another target's cache was used")
		}
	}
}

func TestInvalidRefreshRetainsLastValidSnapshot(t *testing.T) {
	t.Parallel()
	var invalid atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if invalid.Load() {
			_, _ = w.Write([]byte("<html>login</html>"))
			return
		}
		_, _ = w.Write([]byte(testSpec))
	}))
	defer srv.Close()
	svc := Service{Store: Store{Dir: t.TempDir()}, HTTP: srv.Client()}
	target, _ := NewTarget("dev", srv.URL)
	first, err := svc.Acquire(context.Background(), target, "secret", Policy{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	invalid.Store(true)
	if snapshot, err := svc.Acquire(context.Background(), target, "secret", Policy{Refresh: true}); err == nil {
		snapshot.Close()
		t.Fatal("invalid refresh succeeded")
	}
	cached, err := svc.Store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	defer cached.Close()
	if cached.Catalog.RawHash() != first.Catalog.RawHash() {
		t.Fatal("valid snapshot lost")
	}
	receipts := filepath.Join(svc.Store.Dir, "targets", target.Key())
	if err := os.WriteFile(filepath.Join(receipts, "99999999999999999999-invalid.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.Store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if len(recovered.Warnings) == 0 {
		t.Fatal("corrupt snapshot ignored silently")
	}
}

func TestConcurrentStorePublicationNeverRollsBackNewerReceipt(t *testing.T) {
	t.Parallel()
	store := Store{Dir: t.TempDir()}
	target, _ := NewTarget("dev", "http://gateway.test")
	c := testCatalog(t)
	now := time.Now().UTC()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			m := Metadata{Version: 1, Target: target, SourceKind: "gateway", Source: "http://gateway.test/openapi.json",
				FetchedAt: now, VerifiedAt: now.Add(time.Duration(index) * time.Second), RawSHA256: c.RawHash(), ContractSHA256: c.ContractHash(), ParserVersion: ParserVersion}
			if err := store.Save(&Snapshot{Metadata: m, Catalog: c}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	latest, err := store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	defer latest.Close()
	if latest.Metadata.VerifiedAt != now.Add(7*time.Second) {
		t.Fatal("older publication rolled back latest snapshot")
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "blobs", c.RawHash()+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledDiscoveryDoesNotPublish(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	svc := Service{Store: Store{Dir: t.TempDir()}, HTTP: srv.Client()}
	target, _ := NewTarget("dev", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if snapshot, err := svc.Acquire(ctx, target, "secret", Policy{}); err == nil {
		snapshot.Close()
		t.Fatal("canceled discovery succeeded")
	}
	if _, err := svc.Store.Load(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled discovery left a snapshot: %v", err)
	}
}
