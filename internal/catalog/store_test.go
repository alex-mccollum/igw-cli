package catalog

import (
	"context"
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
		if err := publishMetadata(filepath.Join(store.targetDir(target), "current.json"), snapshot.Metadata); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(context.Background(), target); err == nil {
			t.Fatal("invalid cache identity accepted")
		}
	}
}

func TestStoreDoesNotRotateDamagedMetadata(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	target, _ := NewTarget("dev", "http://gateway.test")
	previous := saveTestSnapshot(t, store, target, 1)
	current := saveTestSnapshot(t, store, target, 2)
	path := filepath.Join(store.targetDir(target), "current.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	damaged := strings.Replace(string(raw), current.Metadata.ContractSHA256, strings.Repeat("0", 64), 1)
	if err := os.WriteFile(path, []byte(damaged), 0600); err != nil {
		t.Fatal(err)
	}
	saveTestSnapshot(t, store, target, 3)
	if err := os.WriteFile(path, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), target)
	if err != nil || got.Metadata.RawSHA256 != previous.Metadata.RawSHA256 {
		t.Fatalf("metadata corruption replaced valid fallback: %+v %v", got, err)
	}
}

func TestStoreCollectsInterruptedTemporaryFiles(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	target, _ := NewTarget("dev", "http://gateway.test")
	saveTestSnapshot(t, store, target, 1)
	for _, dir := range []string{store.targetDir(target), filepath.Join(store.targetDir(target), "blobs")} {
		for _, name := range []string{".igw-artifact-abandoned", "unrelated.txt"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	saveTestSnapshot(t, store, target, 2)
	for _, dir := range []string{store.targetDir(target), filepath.Join(store.targetDir(target), "blobs")} {
		if _, err := os.Stat(filepath.Join(dir, ".igw-artifact-abandoned")); !os.IsNotExist(err) {
			t.Fatal("abandoned temporary retained")
		}
		if _, err := os.Stat(filepath.Join(dir, "unrelated.txt")); err != nil {
			t.Fatal("unrelated cache-directory file removed")
		}
	}
}

func TestStoreReusesVerifiedBlob(t *testing.T) {
	for _, source := range []string{"current", "previous"} {
		t.Run(source, func(t *testing.T) {
			store := Store{Dir: t.TempDir()}
			target, _ := NewTarget("dev", "http://gateway.test")
			snapshot := saveTestSnapshot(t, store, target, 1)
			if source == "previous" {
				saveTestSnapshot(t, store, target, 2)
			}
			path := filepath.Join(store.targetDir(target), "blobs", snapshot.Metadata.RawSHA256+".json")
			stamp := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Metadata.VerifiedAt = snapshot.Metadata.VerifiedAt.Add(time.Minute)
			if err := store.Save(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("verified blob was rewritten: %v", err)
			}
			loaded, err := store.Load(context.Background(), target)
			if err != nil || !loaded.Metadata.VerifiedAt.Equal(snapshot.Metadata.VerifiedAt) || loaded.Catalog.RawHash() != snapshot.Catalog.RawHash() {
				t.Fatalf("revalidation metadata not published: %+v %v", loaded, err)
			}
		})
	}
}

func TestStoreRepairsUnusableBlobOnRevalidation(t *testing.T) {
	for _, damage := range []string{"missing", "corrupt"} {
		t.Run(damage, func(t *testing.T) {
			store := Store{Dir: t.TempDir()}
			target, _ := NewTarget("dev", "http://gateway.test")
			snapshot := saveTestSnapshot(t, store, target, 1)
			path := filepath.Join(store.targetDir(target), "blobs", snapshot.Metadata.RawSHA256+".json")
			var err error
			if damage == "missing" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte("broken"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Metadata.VerifiedAt = snapshot.Metadata.VerifiedAt.Add(time.Minute)
			if err := store.Save(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load(context.Background(), target)
			if err != nil || loaded.Catalog.RawHash() != snapshot.Catalog.RawHash() || !loaded.Metadata.VerifiedAt.Equal(snapshot.Metadata.VerifiedAt) {
				t.Fatalf("blob not repaired: %+v %v", loaded, err)
			}
		})
	}
}
