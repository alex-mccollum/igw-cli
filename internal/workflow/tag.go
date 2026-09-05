// Package workflow defines catalog prerequisites shared by execution,
// discovery, and reference qualification. It performs no I/O.
package workflow

import "github.com/alex-mccollum/igw-cli/internal/catalog"

const TagImportOperation = "POST /data/api/v1/tags/import"
const TagExportOperation = "GET /data/api/v1/tags/export"

func TagExport() catalog.Capability {
	return catalog.Capability{ID: "tag.export", Description: "Export tags as JSON or XML", RequiredOperations: []string{TagExportOperation}}
}

func TagImport(verified bool) catalog.Capability {
	if verified {
		return catalog.Capability{ID: "tag.import.verified-json", Description: "Import JSON with Abort, Overwrite, or MergeOverwrite and verify exported properties", RequiredOperations: []string{TagImportOperation, TagExportOperation}}
	}
	return catalog.Capability{ID: "tag.import", Description: "Import XML/CSV or use Rename/Ignore; report acknowledgement without independent readback", RequiredOperations: []string{TagImportOperation}}
}

// TagRequirements returns independent definitions in stable discovery order.
func TagRequirements() []catalog.Capability {
	return []catalog.Capability{TagExport(), TagImport(false), TagImport(true)}
}

func AssessTags(c *catalog.Catalog) ([]catalog.CapabilityAssessment, error) {
	assessments := make([]catalog.CapabilityAssessment, 0)
	for _, requirement := range TagRequirements() {
		assessment, err := c.Assess(requirement)
		if err != nil {
			return nil, err
		}
		assessments = append(assessments, assessment)
	}
	return assessments, nil
}
