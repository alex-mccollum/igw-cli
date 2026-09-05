package referencebuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
	"github.com/alex-mccollum/igw-cli/internal/reference"
)

// Synthetic workflow receipts isolate profile binding; real captures and the
// opt-in matrix runs supply separate observation and workflow evidence.
func profileFixture(t *testing.T) Inputs {
	t.Helper()
	in := fixtureInputs(t)
	path := filepath.Join(in.CaptureDir, "capture.json")
	mutateReceipt(t, path, func(r map[string]any) {
		r["moduleWhitelist"] = []string{moduleprofile.OPCUA}
		inventory := r["moduleInventory"].(map[string]any)
		modules := inventory["modules"].([]any)
		modules = append(modules, map[string]any{"id": "com.inductiveautomation.perspective", "version": "3.3.9", "collection": "healthy", "state": "INACTIVE", "onStartup": "disabled", "shouldUpgrade": false})
		// Use the inventory's canonical typed encoding for its recorded identity.
		b, _ := json.Marshal(modules)
		var typed []moduleprofile.Module
		if err := json.Unmarshal(b, &typed); err != nil {
			t.Fatal(err)
		}
		b, _ = json.Marshal(typed)
		inventory["modules"], inventory["sha256"] = modules, digest(append([]byte("igw-module-inventory/1\n"), b...))
	})
	var captured map[string]any
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &captured); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{in.Lifecycle, in.Resources, in.Transfers, in.Operations} {
		mutateReceipt(t, path, func(r map[string]any) {
			r["moduleWhitelist"] = captured["moduleWhitelist"]
			if path != in.Lifecycle {
				r["moduleInventory"] = captured["moduleInventory"]
			}
		})
	}
	return in
}

func mutateReceipt(t *testing.T, path string, change func(map[string]any)) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	change(value)
	b, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildCoreProfileKeepsCompleteInventory(t *testing.T) {
	in := profileFixture(t)
	m, err := Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if m.ModuleProfile == nil || m.ModuleProfile.Name != "core-opcua" || m.ModuleProfile.Policy != moduleprofile.Policy || len(m.Modules) != 2 || m.Modules[1].State != "INACTIVE" {
		t.Fatal("reference lost explicit inactive modules")
	}
	loaded, c, err := reference.OpenCatalog(context.Background(), in.Out)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if loaded.ModuleProfile.Name != "core-opcua" {
		t.Fatal("profile lost during offline readback")
	}
	if reference.Directory(in.Out).Summary(loaded).ModuleProfile.Name != "core-opcua" {
		t.Fatal("profile omitted from discovery summary")
	}
}

func TestBuildRejectsMismatchedProfileReceipts(t *testing.T) {
	for _, kind := range []string{"capture", "lifecycle", "resources", "transfers", "operations"} {
		t.Run(kind, func(t *testing.T) {
			in := profileFixture(t)
			path := map[string]string{"capture": filepath.Join(in.CaptureDir, "capture.json"), "lifecycle": in.Lifecycle, "resources": in.Resources, "transfers": in.Transfers, "operations": in.Operations}[kind]
			mutateReceipt(t, path, func(r map[string]any) { delete(r, "moduleWhitelist") })
			if _, err := Build(context.Background(), in); err == nil {
				t.Fatal("mismatched module selection qualified")
			}
			if _, err := os.Stat(in.Out); !os.IsNotExist(err) {
				t.Fatal("failed qualification published a directory")
			}
		})
	}
}

func TestReferenceRejectsProfileRelabeling(t *testing.T) {
	for _, mode := range []string{"default label", "unknown policy", "wrong enabled", "omitted profile", "filtered modules", "changed state", "changed hash"} {
		t.Run(mode, func(t *testing.T) {
			in := profileFixture(t)
			if _, err := Build(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			mutateReceipt(t, filepath.Join(in.Out, "reference.json"), func(m map[string]any) {
				profile := m["moduleProfile"].(map[string]any)
				switch mode {
				case "default label":
					profile["name"] = "image-defaults"
					delete(profile, "enabledModules")
				case "unknown policy":
					profile["policy"] = "future"
				case "wrong enabled":
					profile["enabledModules"] = []string{"com.inductiveautomation.perspective"}
				case "omitted profile":
					delete(m, "moduleProfile")
				case "filtered modules":
					delete(m, "moduleProfile")
					m["modules"] = m["modules"].([]any)[:1]
				case "changed state":
					m["modules"].([]any)[1].(map[string]any)["state"] = "ACTIVE"
				case "changed hash":
					m["moduleInventorySha256"] = m["catalog"].(map[string]any)["rawSha256"]
				}
			})
			if _, err := reference.Read(context.Background(), in.Out); err == nil {
				t.Fatal("manifest changed or omitted the observed module profile")
			}
		})
	}
}

func TestReferenceRetainsHistoricalActiveWhitelist(t *testing.T) {
	in := fixtureInputs(t)
	m, err := Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// Before module profiles, all-active inventories qualified regardless of
	// whether startup used an explicit whitelist. Model that historical format.
	m.ModuleProfile = nil
	path := filepath.Join(in.Out, "capture.json")
	mutateReceipt(t, path, func(c map[string]any) {
		c["moduleWhitelist"] = []string{moduleprofile.OPCUA}
	})
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range m.Files {
		if m.Files[i].Path == "capture.json" {
			m.Files[i].SHA256, m.Files[i].Bytes = digest(b), int64(len(b))
		}
	}
	b, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in.Out, "reference.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := reference.Read(context.Background(), in.Out)
	if err != nil || loaded.ModuleProfile != nil {
		t.Fatalf("historical active whitelist was rejected or relabeled: %v", err)
	}
}
