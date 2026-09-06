package reference

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestBundledReferenceExportPreservesExactEvidence(t *testing.T) {
	ctx := context.Background()
	items, err := List(ctx)
	want := []string{"ignition-8.3.0-core", "ignition-8.3.0-defaults", "ignition-8.3.9-core", "ignition-8.3.9-defaults"}
	var names []string
	for _, item := range items {
		names = append(names, item.Selector)
		if item.SourceKind != "reference" || item.Origin != "bundled" {
			t.Fatalf("incorrect reference source: %+v", item)
		}
		active := 32
		if strings.HasSuffix(item.Selector, "-core") {
			active = 1
			if item.ModuleProfile == nil || item.ModuleProfile.Name != "core-opcua" {
				t.Fatalf("core reference lost its observed profile: %+v", item)
			}
		} else if item.ModuleProfile != nil {
			t.Fatal("historical reference was relabeled with a module profile")
		}
		if item.ModuleCount != 32 || item.ActiveModuleCount != active {
			t.Fatalf("module counts do not reflect the inventory: %+v", item)
		}
	}
	if err != nil || !slices.Equal(names, want) {
		t.Fatalf("unexpected bundled references: %+v, %v", items, err)
	}
	for _, item := range items {
		t.Run(item.Selector, func(t *testing.T) {
			bundle := Select(item.Selector)
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
				want, err := bundle.read(ctx, name, fileLimit(name))
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
			if err := os.WriteFile(filepath.Join(dir, "openapi.json.gz"), []byte("tampered"), 0600); err != nil {
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
		})
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
