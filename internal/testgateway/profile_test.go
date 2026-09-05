package testgateway

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

func TestAcceptanceProfileConfigurationAndObservation(t *testing.T) {
	cfg, err := ProfileConfig("selected-image", "selected-docker", "core-opcua")
	if err != nil || cfg.Image != "selected-image" || cfg.Docker != "selected-docker" || len(cfg.Modules) != 1 || cfg.Modules[0] != moduleprofile.OPCUA {
		t.Fatal("profile was not applied to the Gateway configuration")
	}
	if _, err := ProfileConfig("image", "docker", "typo"); err == nil {
		t.Fatal("unknown profile accepted")
	}
	b, err := os.ReadFile("../moduleprofile/testdata/ignition-8.3.9-core/capture.json")
	if err != nil {
		t.Fatal(err)
	}
	var captured Evidence
	if err := json.Unmarshal(b, &captured); err != nil {
		t.Fatal(err)
	}
	s := &Session{Modules: cfg.Modules, ModuleInventory: captured.ModuleInventory}
	if err := s.ValidateModuleProfile(); err != nil {
		t.Fatal(err)
	}
	s.Modules = nil
	if s.ValidateModuleProfile() == nil {
		t.Fatal("acceptance ignored an incorrectly realized profile")
	}
}
