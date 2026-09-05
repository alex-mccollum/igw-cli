package moduleprofile

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func capturedInventory(t *testing.T, version string) *Inventory {
	t.Helper()
	dir := filepath.Join("testdata", "ignition-"+version+"-core")
	b, err := os.ReadFile(filepath.Join(dir, "capture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Validated, Cleanup bool
		RawSHA256          string
		ModuleWhitelist    []string
		ModuleInventory    *Inventory
	}
	if json.Unmarshal(b, &receipt) != nil || !receipt.Validated || !receipt.Cleanup {
		t.Fatal("incomplete captured fixture")
	}
	p, err := FromWhitelist(receipt.ModuleWhitelist)
	if err != nil || p.Name != "core-opcua" {
		t.Fatal("unexpected captured selection")
	}
	f, err := os.Open(filepath.Join(dir, "openapi.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, 32<<20))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != receipt.RawSHA256 {
		t.Fatal("retained document differs from original capture")
	}
	return receipt.ModuleInventory
}

func rehash(in *Inventory) {
	b, _ := json.Marshal(in.Modules)
	h := sha256.Sum256(append([]byte("igw-module-inventory/1\n"), b...))
	in.SHA256 = hex.EncodeToString(h[:])
}

func TestCapturedCoreProfilesPreserveInactiveModules(t *testing.T) {
	core, _ := Select("core-opcua")
	defaults, _ := Select("image-defaults")
	for _, version := range []string{"8.3.0", "8.3.9"} {
		t.Run(version, func(t *testing.T) {
			in := capturedInventory(t, version)
			if err := core.ValidateInventory(in); err != nil {
				t.Fatal(err)
			}
			active, inactive := 0, 0
			for _, m := range in.Modules {
				if m.State == "ACTIVE" {
					active++
					if m.ID != OPCUA {
						t.Fatal("unexpected active module")
					}
				}
				if m.State == "INACTIVE" {
					inactive++
				}
			}
			if active != 1 || inactive != 31 {
				t.Fatal("incomplete observed module inventory")
			}
			if err := defaults.ValidateInventory(in); err == nil {
				t.Fatal("core capture qualified as image defaults")
			}
		})
	}
}

func TestProfileRejectsUnexpectedModuleStates(t *testing.T) {
	core, _ := Select("core-opcua")
	for _, mode := range []string{"extra active", "faulted", "quarantined", "wrong startup", "upgrade", "missing selected", "selected inactive", "duplicate", "checksum"} {
		t.Run(mode, func(t *testing.T) {
			in := capturedInventory(t, "8.3.9")
			switch mode {
			case "extra active":
				in.Modules[0].State, in.Modules[0].OnStartup = "ACTIVE", "enabled"
			case "faulted":
				in.Modules[0].State = "FAULTED"
			case "quarantined":
				in.Modules[0].Collection = "quarantined"
			case "wrong startup":
				in.Modules[0].OnStartup = "enabled"
			case "upgrade":
				v := true
				in.Modules[0].ShouldUpgrade = &v
			case "missing selected":
				for i, m := range in.Modules {
					if m.ID == OPCUA {
						in.Modules = append(in.Modules[:i], in.Modules[i+1:]...)
						break
					}
				}
			case "selected inactive":
				for i, m := range in.Modules {
					if m.ID == OPCUA {
						in.Modules[i].State, in.Modules[i].OnStartup = "INACTIVE", "disabled"
					}
				}
			case "duplicate":
				in.Modules[1].ID = in.Modules[0].ID
			}
			rehash(in)
			if mode == "checksum" {
				in.SHA256 = strings.Repeat("0", 64)
			}
			if err := core.ValidateInventory(in); err == nil {
				t.Fatal("unexpected module profile was qualified")
			}
		})
	}
}

func TestProfileSelectionIsExplicitAndVersioned(t *testing.T) {
	if _, err := Select("core"); err == nil {
		t.Fatal("accepted unknown profile")
	}
	if _, err := FromWhitelist([]string{OPCUA, "com.inductiveautomation.perspective"}); err == nil {
		t.Fatal("qualified an unreviewed whitelist")
	}
	p, _ := Select("core-opcua")
	p.Policy = "future"
	if p.Validate() == nil {
		t.Fatal("accepted unknown policy")
	}
	p, _ = Select("core-opcua")
	p.EnabledModules[0] = "com.inductiveautomation.perspective"
	if p.Validate() == nil {
		t.Fatal("accepted relabeled selection")
	}
	next, _ := Select("core-opcua")
	if next.EnabledModules[0] != OPCUA {
		t.Fatal("selection mutated shared defaults")
	}
}
