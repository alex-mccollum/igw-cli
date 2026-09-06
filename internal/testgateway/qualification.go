package testgateway

import (
	"errors"
	"slices"
	"sort"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

// TagRoundTripAvailable validates policy 2's two reviewed capability shapes:
// both transfer routes advertised, or both absent. Partial API surfaces require
// additional qualification coverage; they are not silently treated as absent.
func TagRoundTripAvailable(assessments []catalog.CapabilityAssessment) (bool, error) {
	requirements := workflow.TagRequirements()
	if len(assessments) != len(requirements) {
		return false, errors.New("incomplete tag capability evidence")
	}
	status := assessments[0].Status
	if status != "advertised" && status != "unavailable" {
		return false, errors.New("invalid tag capability status")
	}
	for index, requirement := range requirements {
		sort.Strings(requirement.RequiredOperations)
		got := assessments[index]
		if got.ID != requirement.ID || !slices.Equal(got.RequiredOperations, requirement.RequiredOperations) {
			return false, errors.New("tag capability prerequisites differ from the qualification policy")
		}
		if got.Status != status || (status == "advertised" && len(got.MissingOperations) != 0) || (status == "unavailable" && !slices.Equal(got.MissingOperations, requirement.RequiredOperations)) {
			return false, errors.New("partial or inconsistent tag API requires additional qualification coverage")
		}
	}
	return status == "advertised", nil
}

func NewQualification(binaryHash string, capabilities []catalog.CapabilityAssessment) (reference.Qualification, error) {
	available, err := TagRoundTripAvailable(capabilities)
	if err != nil {
		return reference.Qualification{}, err
	}
	q := reference.Qualification{Policy: reference.QualificationPolicy, TestBinarySHA256: binaryHash, Scopes: reference.QualificationScopes(), Capabilities: capabilities}
	if !available {
		q.Scopes = slices.DeleteFunc(q.Scopes, func(scope string) bool { return scope == "tags/memory-json" })
		q.UnavailableScopes = []string{"tags/memory-json"}
	}
	return q, nil
}

// SameCapabilities compares prerequisite evidence; descriptive wording is not
// an assertion about which operations are available.
func SameCapabilities(a, b []catalog.CapabilityAssessment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Status != b[i].Status || !slices.Equal(a[i].RequiredOperations, b[i].RequiredOperations) || !slices.Equal(a[i].MissingOperations, b[i].MissingOperations) {
			return false
		}
	}
	return true
}
