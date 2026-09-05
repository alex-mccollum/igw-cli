package tag

import "github.com/alex-mccollum/igw-cli/internal/catalog"

const importOperation = "POST /data/api/v1/tags/import"
const exportOperation = "GET /data/api/v1/tags/export"

func exportCapability() catalog.Capability {
	return catalog.Capability{ID: "tag.export", Description: "Export tags as JSON or XML", RequiredOperations: []string{exportOperation}}
}

func importCapability(verified bool) catalog.Capability {
	if verified {
		return catalog.Capability{ID: "tag.import.verified-json", Description: "Import JSON with Abort, Overwrite, or MergeOverwrite and verify exported properties", RequiredOperations: []string{importOperation, exportOperation}}
	}
	return catalog.Capability{ID: "tag.import", Description: "Import XML/CSV or use Rename/Ignore; report acknowledgement without independent readback", RequiredOperations: []string{importOperation}}
}

// Capabilities supplies discovery and the same requirements enforced by the
// typed workflows. Callers receive independent slices, not a mutable registry.
func Capabilities() []catalog.Capability {
	return []catalog.Capability{exportCapability(), importCapability(false), importCapability(true)}
}

func (r ImportRequest) verifiedJSON() bool {
	return r.Format == "json" && (r.CollisionPolicy == "Abort" || r.CollisionPolicy == "Overwrite" || r.CollisionPolicy == "MergeOverwrite")
}
