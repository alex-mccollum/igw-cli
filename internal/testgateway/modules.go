package testgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

// Module records observed deployment state, not an assertion of module health.
// Ignition's "healthy" collection can include unloaded or faulted modules.
type Module = moduleprofile.Module
type ModuleInventory = moduleprofile.Inventory

const modulePageSize = 100
const maxModules = 2000

// readModuleInventory reads both documented collections. Bounded pagination,
// explicit totals, unique IDs, and a stable sort prevent a partial inventory
// from being mistaken for a complete module profile.
func (s *Session) readModuleInventory(ctx context.Context, client *http.Client) (*ModuleInventory, error) {
	modules := []Module{}
	seen := map[string]bool{}
	for _, collection := range []string{"healthy", "quarantined"} {
		total := -1
		for offset := 0; ; {
			query := url.Values{"limit": {strconv.Itoa(modulePageSize)}, "offset": {strconv.Itoa(offset)}, "sortBy": {"asc(id)"}}
			address := s.URL + "/data/api/v1/modules/" + collection + "?" + query.Encode()
			b, _, err := sessionRequest(ctx, client, http.MethodGet, address, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("module inventory %s: %w", collection, err)
			}
			page, count, err := modulePage(b, collection, offset)
			if err != nil {
				return nil, err
			}
			if total != -1 && total != count {
				return nil, errors.New("module inventory changed during pagination")
			}
			total = count
			for _, module := range page {
				if seen[module.ID] {
					return nil, errors.New("module inventory contains duplicate or moving module IDs")
				}
				seen[module.ID] = true
				modules = append(modules, module)
				if len(modules) > maxModules {
					return nil, errors.New("module inventory exceeds 2000 modules")
				}
			}
			offset += len(page)
			if offset == total {
				break
			}
		}
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].ID < modules[j].ID })
	b, err := json.Marshal(modules)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append([]byte("igw-module-inventory/1\n"), b...))
	return &ModuleInventory{Version: 1, ObservedAt: time.Now().UTC(), SHA256: hex.EncodeToString(sum[:]), Modules: modules}, nil
}

func modulePage(raw []byte, collection string, offset int) ([]Module, int, error) {
	bad := errors.New("module inventory returned an incomplete or invalid page")
	if err := jsonvalue.Validate(raw); err != nil {
		return nil, 0, bad
	}
	var page struct {
		Items *[]struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Version       string `json:"version"`
			State         string `json:"state"`
			OnStartup     string `json:"onStartup"`
			ShouldUpgrade *bool  `json:"shouldUpgrade"`
		} `json:"items"`
		Metadata struct {
			Total    *int `json:"total"`
			Matching *int `json:"matching"`
			Limit    *int `json:"limit"`
			Offset   *int `json:"offset"`
		} `json:"metadata"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Items == nil || page.Metadata.Total == nil || page.Metadata.Matching == nil || page.Metadata.Limit == nil || page.Metadata.Offset == nil {
		return nil, 0, bad
	}
	total := *page.Metadata.Total
	if total < 0 || total > maxModules || *page.Metadata.Matching != total || *page.Metadata.Limit != modulePageSize || *page.Metadata.Offset != offset || offset > total || len(*page.Items) > modulePageSize || offset+len(*page.Items) > total || (len(*page.Items) == 0 && offset != total) {
		return nil, 0, bad
	}
	modules := make([]Module, 0, len(*page.Items))
	for _, item := range *page.Items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Version) == "" || len(item.ID) > 256 || len(item.Version) > 256 || len(item.Name) > 512 || len(item.State) > 256 || len(item.OnStartup) > 256 {
			return nil, 0, bad
		}
		modules = append(modules, Module{ID: item.ID, Name: item.Name, Version: item.Version, Collection: collection, State: item.State, OnStartup: item.OnStartup, ShouldUpgrade: item.ShouldUpgrade})
	}
	return modules, total, nil
}
