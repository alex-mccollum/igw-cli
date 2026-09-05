package catalog

import (
	"errors"
	"sort"
	"strings"
)

// Capability is a workflow's catalog prerequisite, owned by that workflow.
// Advertised operations do not establish permission or validate instance data.
type Capability struct {
	ID                 string   `json:"id"`
	Description        string   `json:"description"`
	RequiredOperations []string `json:"requiredOperations"`
}

type CapabilityAssessment struct {
	Capability
	Status            string   `json:"status"`
	MissingOperations []string `json:"missingOperations"`
}

// Assess uses exact method/path keys. Version labels, operation aliases, and
// offline qualification receipts cannot substitute for advertised operations.
func (c *Catalog) Assess(requirement Capability) (CapabilityAssessment, error) {
	if c == nil || requirement.ID == "" || len(requirement.RequiredOperations) == 0 {
		return CapabilityAssessment{}, errors.New("invalid workflow capability definition")
	}
	requirement.RequiredOperations = append([]string(nil), requirement.RequiredOperations...)
	sort.Strings(requirement.RequiredOperations)
	out := CapabilityAssessment{Capability: requirement, Status: "advertised", MissingOperations: []string{}}
	for index, key := range requirement.RequiredOperations {
		method, path, ok := strings.Cut(key, " ")
		validMethod := false
		switch method {
		case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE":
			validMethod = true
		}
		if !ok || !strings.HasPrefix(path, "/") || !validMethod || (index > 0 && key == requirement.RequiredOperations[index-1]) {
			return CapabilityAssessment{}, errors.New("invalid workflow capability operation key")
		}
		if _, ok := c.ops[key]; !ok {
			out.MissingOperations = append(out.MissingOperations, key)
		}
	}
	if len(out.MissingOperations) > 0 {
		out.Status = "unavailable"
	}
	return out, nil
}
