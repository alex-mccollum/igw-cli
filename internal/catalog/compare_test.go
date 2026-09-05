package catalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompareSeparatesSharedContractsAndDocumentation(t *testing.T) {
	before := testCatalog(t)
	for _, tc := range []struct {
		name, document                       string
		contractEqual, documentEqual, shared bool
		added, removed, changed              []string
	}{
		{"identical", testSpec, true, true, false, []string{}, []string{}, []string{}},
		{"annotation", strings.Replace(testSpec, `"operationId":"item"`, `"operationId":"item","description":"documentation only"`, 1), true, false, true, []string{}, []string{}, []string{"GET /items/{name}"}},
		{"shared large integer", strings.ReplaceAll(testSpec, "9007199254740993", "9007199254740992"), false, false, true, []string{}, []string{}, []string{}},
		{"inherited path constraint", strings.Replace(testSpec, `"schema":{"type":"string"}`, `"schema":{"type":"string","minLength":1}`, 1), false, false, true, []string{}, []string{}, []string{}},
		{"route replacement", strings.Replace(testSpec, `"/health"`, `"/ready"`, 1), false, false, true, []string{"GET /ready"}, []string{"GET /health"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, err := Parse([]byte(tc.document))
			if err != nil {
				t.Fatal(err)
			}
			defer after.Close()
			got := Compare(before, after)
			if got.BeforeIdentity != before.Identity() || got.AfterIdentity != after.Identity() || got.ContractEqual != tc.contractEqual || got.DocumentEqual != tc.documentEqual || got.SharedOrPathDocumentChanged != tc.shared || !reflect.DeepEqual(got.Added, tc.added) || !reflect.DeepEqual(got.Removed, tc.removed) || !reflect.DeepEqual(got.ChangedOperationDocuments, tc.changed) {
				t.Fatalf("unexpected comparison: %+v", got)
			}
			if (got.Compatibility == "unchanged_under_policy") != tc.contractEqual {
				t.Fatal("compatibility overstates evidence")
			}
			b, err := json.Marshal(got)
			if err != nil || strings.Contains(string(b), ":null") {
				t.Fatalf("comparison arrays must be stable JSON arrays: %s %v", b, err)
			}
		})
	}
}
