package reference

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBundledReferenceExportPreservesExactEvidence(t *testing.T) {
	ctx := context.Background()
	items, err := List(ctx)
	if err != nil || len(items) != 1 || items[0].Selector != "ignition-8.3.9-defaults" || items[0].SourceKind != "reference" || items[0].Origin != "bundled" {
		t.Fatalf("unexpected bundled references: %+v, %v", items, err)
	}
	bundle := Select(items[0].Selector)
	m, err := bundle.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "exported")
	exported, err := bundle.Export(ctx, dir, m.Catalog)
	if err != nil || !reflect.DeepEqual(exported, m) {
		t.Fatalf("export changed manifest: %v", err)
	}
	for _, name := range append(RequiredFiles(), "reference.json") {
		want, err := bundle.read(ctx, name, MaxManifestBytes)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("export changed original %s: %v", name, err)
		}
	}
	local := Select(dir)
	loaded, err := local.Read(ctx)
	if err != nil || local.Summary(loaded).Origin != "directory" || !reflect.DeepEqual(loaded, m) {
		t.Fatalf("export was not independently readable: %v", err)
	}
	if _, err := bundle.Export(ctx, dir, m.Catalog); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing directory was not preserved: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.json"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := local.Read(ctx); err == nil {
		t.Fatal("accepted corrupt qualification evidence")
	}
	badOut := filepath.Join(t.TempDir(), "corrupt-copy")
	if _, err := local.Export(ctx, badOut, m.Catalog); err == nil {
		t.Fatal("exported corrupt qualification evidence")
	}
	if _, err := os.Stat(badOut); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("export created output before checking evidence")
	}
}

func TestReferenceExportRefusesChangedIdentityAndCancellation(t *testing.T) {
	ctx := context.Background()
	bundle := Select("ignition-8.3.9-defaults")
	m, err := bundle.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wrong := m.Catalog
	wrong.ContractSHA256 = "different"
	dir := filepath.Join(t.TempDir(), "output")
	if _, err := bundle.Export(ctx, dir, wrong); err == nil {
		t.Fatal("export ignored reviewed identity")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := bundle.Export(ctx, dir, m.Catalog); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled export: %v", err)
	}
	if _, err := List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled listing: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused export created output")
	}
	if _, _, err := (Bundle{}).OpenCatalog(context.Background()); err == nil {
		t.Fatal("zero bundle unexpectedly opened")
	}
}
