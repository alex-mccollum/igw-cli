package catalog

import (
	"reflect"
	"testing"
)

func TestCapabilityRequiresExactOperations(t *testing.T) {
	c := testCatalog(t)
	op := c.Operations()[0]
	requirement := Capability{ID: "fixture", RequiredOperations: []string{op.Key, "DELETE /missing"}}
	before := append([]string(nil), requirement.RequiredOperations...)
	got, err := c.Assess(requirement)
	if err != nil || got.Status != "unavailable" || !reflect.DeepEqual(got.MissingOperations, []string{"DELETE /missing"}) || !reflect.DeepEqual(requirement.RequiredOperations, before) {
		t.Fatalf("incorrect capability assessment: %+v %v", got, err)
	}
	requirement.RequiredOperations = []string{op.Key}
	got, err = c.Assess(requirement)
	if err != nil || got.Status != "advertised" || got.MissingOperations == nil || len(got.MissingOperations) != 0 {
		t.Fatal("advertised prerequisite missing")
	}
	for _, keys := range [][]string{nil, {op.Key, op.Key}, {"alias"}, {"GET PUT /missing"}, {"get /missing"}} {
		requirement.RequiredOperations = keys
		if _, err := c.Assess(requirement); err == nil {
			t.Fatalf("invalid requirement accepted: %v", keys)
		}
	}
}
