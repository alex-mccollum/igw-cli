package testgateway_test

import (
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

// Discovery also includes operational capabilities. The reference policy binds
// exactly the reviewed tag subset, preserving original assessments unchanged.
func qualificationTagCapabilities(t *testing.T, all []catalog.CapabilityAssessment) []catalog.CapabilityAssessment {
	t.Helper()
	var tags []catalog.CapabilityAssessment
	for _, requirement := range workflow.TagRequirements() {
		for _, assessment := range all {
			if assessment.ID == requirement.ID {
				tags = append(tags, assessment)
			}
		}
	}
	if _, err := testgateway.TagRoundTripAvailable(tags); err != nil {
		t.Fatal(err)
	}
	return tags
}

func TestQualificationTagCapabilitiesFromDiscovery(t *testing.T) {
	c, err := catalog.Parse([]byte(`{"openapi":"3.1.0","info":{"title":"test","version":"1"},"paths":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	all, err := workflow.Assess(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) <= len(workflow.TagRequirements()) {
		t.Fatal("fixture does not include operational capabilities")
	}
	if _, err := testgateway.TagRoundTripAvailable(all); err == nil {
		t.Fatal("tag policy accepted unrelated capabilities")
	}
	tags := qualificationTagCapabilities(t, all)
	available, err := testgateway.TagRoundTripAvailable(tags)
	if err != nil || available {
		t.Fatal("absent tag routes not recognized")
	}
}
