// Package moduleprofile defines reviewed contributor module selections and
// validates their complete observed inventory without Gateway or Docker I/O.
package moduleprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const Policy = "igw-module-profile/1"
const OPCUA = "com.inductiveautomation.opcua"

type Selection struct {
	Policy         string   `json:"policy"`
	Name           string   `json:"name"`
	EnabledModules []string `json:"enabledModules,omitempty"`
}

// Module preserves observed deployment state. The Gateway's healthy collection
// can contain inactive or faulted modules; its name alone is not health proof.
type Module struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Version       string `json:"version"`
	Collection    string `json:"collection"`
	State         string `json:"state,omitempty"`
	OnStartup     string `json:"onStartup,omitempty"`
	ShouldUpgrade *bool  `json:"shouldUpgrade,omitempty"`
}

type Inventory struct {
	Version    int       `json:"version"`
	ObservedAt time.Time `json:"observedAt"`
	SHA256     string    `json:"sha256"`
	Modules    []Module  `json:"modules"`
}

func Select(name string) (Selection, error) {
	switch name {
	case "", "image-defaults":
		return Selection{Policy: Policy, Name: "image-defaults"}, nil
	case "core-opcua":
		return Selection{Policy: Policy, Name: name, EnabledModules: []string{OPCUA}}, nil
	default:
		return Selection{}, errors.New("module profile must be image-defaults or core-opcua")
	}
}

func FromWhitelist(modules []string) (Selection, error) {
	if len(modules) == 0 {
		return Select("image-defaults")
	}
	if len(modules) == 1 && modules[0] == OPCUA {
		return Select("core-opcua")
	}
	return Selection{}, errors.New("module whitelist has no reviewed qualification profile")
}

func (s Selection) Validate() error {
	want, err := Select(s.Name)
	if err != nil || s.Name == "" || s.Policy != Policy || !slices.Equal(s.EnabledModules, want.EnabledModules) {
		return errors.New("invalid or unsupported module profile")
	}
	return nil
}

// ValidateInventory requires every selected module to be active and every
// excluded module to be explicitly inactive/disabled. Nothing is filtered out.
func (s Selection) ValidateInventory(in *Inventory) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if in == nil || in.Version != 1 || in.ObservedAt.IsZero() || len(in.Modules) == 0 || len(in.Modules) > 2000 {
		return errors.New("a complete observed module inventory is required")
	}
	last := ""
	selected := 0
	for _, m := range in.Modules {
		if !strings.HasPrefix(m.ID, "com.inductiveautomation.") || m.ID <= last || len(m.ID) > 256 || strings.TrimSpace(m.Version) == "" || len(m.Version) > 256 || len(m.Name) > 512 || m.Collection != "healthy" || m.ShouldUpgrade == nil || *m.ShouldUpgrade {
			return errors.New("module inventory requires sorted unique first-party modules without quarantine or pending upgrades")
		}
		state, startup := "INACTIVE", "disabled"
		if len(s.EnabledModules) == 0 || slices.Contains(s.EnabledModules, m.ID) {
			state, startup = "ACTIVE", "enabled"
			selected++
		}
		if m.State != state || m.OnStartup != startup {
			return errors.New("observed module state does not match the requested profile")
		}
		last = m.ID
	}
	if len(s.EnabledModules) > 0 && selected != len(s.EnabledModules) {
		return errors.New("selected module is missing from the observed inventory")
	}
	b, err := json.Marshal(in.Modules)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(append([]byte("igw-module-inventory/1\n"), b...))
	if hex.EncodeToString(sum[:]) != in.SHA256 {
		return errors.New("module inventory checksum does not match its observations")
	}
	return nil
}
