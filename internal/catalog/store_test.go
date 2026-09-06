package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func saveTestSnapshot(t *testing.T, store Store, target Target, n int) *Snapshot {
	t.Helper()
	raw := testSpec
	// Whitespace changes representation identity without changing the contract.
	c, err := Parse([]byte(raw + strings.Repeat(" ", n)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 6, 0, 0, n, 0, time.UTC)
	snapshot := &Snapshot{Catalog: c, Metadata: Metadata{Version: SnapshotVersion, Target: target, SourceKind: "gateway", Source: target.URL + "/openapi.json", FetchedAt: now, VerifiedAt: now, RawSHA256: c.RawHash(), ContractSHA256: c.ContractHash()}}
	if err := store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestStoreBoundsHistoryAndRecoversCorruption(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	target, _ := NewTarget("dev", "http://gateway.test")
	var previous, current *Snapshot
	for n := 1; n <= 6; n++ {
		previous, current = current, saveTestSnapshot(t, store, target, n)
	}
	dir := store.targetDir(target)
	blobs, _ := os.ReadDir(filepath.Join(dir, "blobs"))
	if len(blobs) != 2 {
		t.Fatalf("unbounded retained blobs: %d", len(blobs))
	}
	// Damage the newest document, then publish a new valid snapshot. The valid
	// fallback must survive both the read and publication of the replacement.
	if err := os.WriteFile(filepath.Join(dir, "blobs", current.Metadata.RawSHA256+".json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), target)
	if err != nil || got.Metadata.RawSHA256 != previous.Metadata.RawSHA256 || len(got.Warnings) != 1 {
		t.Fatalf("corrupt current recovery: %+v %v", got, err)
	}
	saveTestSnapshot(t, store, target, 7)
	if err := os.WriteFile(filepath.Join(dir, "current.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load(context.Background(), target)
	if err != nil || got.Metadata.RawSHA256 != previous.Metadata.RawSHA256 {
		t.Fatalf("valid previous lost: %+v %v", got, err)
	}
}

func TestStoreInterruptedPublicationRetainsPrevious(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	target, _ := NewTarget("dev", "http://gateway.test")
	previous := saveTestSnapshot(t, store, target, 1)
	next := saveTestSnapshot(t, store, target, 2)
	dir := store.targetDir(target)
	// Model an unusable destination at the final publication boundary.
	if err := os.Remove(filepath.Join(dir, "current.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "current.json"), 0700); err != nil {
		t.Fatal(err)
	}
	next.Metadata.VerifiedAt = next.Metadata.VerifiedAt.Add(time.Minute)
	if err := store.Save(context.Background(), next); err == nil {
		t.Fatal("publication unexpectedly succeeded")
	}
	got, err := store.Load(context.Background(), target)
	if err != nil || got.Metadata.RawSHA256 != previous.Metadata.RawSHA256 {
		t.Fatalf("failed publication lost fallback: %+v %v", got, err)
	}
	if err := os.Remove(filepath.Join(dir, "current.json")); err != nil {
		t.Fatal(err)
	}
	saveTestSnapshot(t, store, target, 3)
	blobs, _ := os.ReadDir(filepath.Join(dir, "blobs"))
	if len(blobs) != 2 {
		t.Fatalf("orphan was not collected: %d", len(blobs))
	}
}

func TestStoreRejectsObsoleteAndInvalidIdentities(t *testing.T) {
	for _, mutate := range []func(*Metadata){
		func(m *Metadata) { m.Version = 1 },
		func(m *Metadata) { m.Version = 2 },
		func(m *Metadata) { m.ContractPolicy = "igw-contract/1" },
		func(m *Metadata) { m.DocumentSHA256 = strings.Repeat("0", 64) },
		func(m *Metadata) { m.ContractSHA256 = strings.Repeat("0", 64) },
		func(m *Metadata) { m.Source = "http://another.test/openapi.json" },
	} {
		store := Store{Dir: t.TempDir()}
		target, _ := NewTarget("dev", "http://gateway.test")
		snapshot := saveTestSnapshot(t, store, target, 1)
		mutate(&snapshot.Metadata)
		raw, _ := json.Marshal(snapshot.Metadata)
		if err := os.WriteFile(filepath.Join(store.targetDir(target), "current.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(context.Background(), target); err == nil {
			t.Fatal("invalid cache identity accepted")
		}
	}
}
