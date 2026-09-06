package catalog

import (
	"bytes"
	"encoding/json"
	"sort"
)

// Comparison separates document changes from changes under the versioned
// contract policy. Neither result proves runtime or backward compatibility.
type Comparison struct {
	BeforeIdentity              Identity `json:"beforeIdentity"`
	AfterIdentity               Identity `json:"afterIdentity"`
	ContractEqual               bool     `json:"contractEqual"`
	DocumentEqual               bool     `json:"documentEqual"`
	Added                       []string `json:"added"`
	Removed                     []string `json:"removed"`
	ChangedOperationDocuments   []string `json:"changedOperationDocuments"`
	SharedOrPathDocumentChanged bool     `json:"sharedOrPathDocumentChanged"`
	Compatibility               string   `json:"compatibility"`
}

// Compare is shared by local inspection and reference-update qualification.
// The caller owns the already-validated catalogs and their lifetimes.
func Compare(before, after *Catalog) Comparison {
	result := Comparison{
		BeforeIdentity: before.Identity(), AfterIdentity: after.Identity(),
		ContractEqual: before.ContractHash() == after.ContractHash(),
		DocumentEqual: before.DocumentHash() == after.DocumentHash(),
		Added:         []string{}, Removed: []string{}, ChangedOperationDocuments: []string{},
		Compatibility: "requires_review",
	}
	if result.ContractEqual {
		result.Compatibility = "unchanged_under_policy"
	}
	if result.DocumentEqual {
		return result
	}
	// Read the immutable private indexes without cloning both large vendor
	// documents and every operation definition merely to compare them.
	for _, op := range after.ops {
		old, ok := before.ops[op.Key]
		if !ok {
			result.Added = append(result.Added, op.Key)
		} else if !sameDocumentJSON(old.Definition, op.Definition) {
			result.ChangedOperationDocuments = append(result.ChangedOperationDocuments, op.Key)
		}
	}
	for _, op := range before.ops {
		if _, ok := after.ops[op.Key]; !ok {
			result.Removed = append(result.Removed, op.Key)
		}
	}
	sort.Strings(result.Added)
	sort.Strings(result.Removed)
	sort.Strings(result.ChangedOperationDocuments)
	for _, field := range []string{"components", "security", "paths"} {
		old, next := before.root[field], after.root[field]
		if field == "paths" {
			old, _ = json.Marshal(before.paths)
			next, _ = json.Marshal(after.paths)
		}
		if !sameDocumentJSON(old, next) {
			result.SharedOrPathDocumentChanged = true
		}
	}
	return result
}

func sameDocumentJSON(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	decode := func(raw []byte) []byte {
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if d.Decode(&value) != nil {
			return nil
		}
		normalized, _ := json.Marshal(value)
		return normalized
	}
	return bytes.Equal(decode(a), decode(b))
}
