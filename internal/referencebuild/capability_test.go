package referencebuild

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

func withoutTagRoutes(t *testing.T) string {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(syntheticDocument), &root); err != nil {
		t.Fatal(err)
	}
	paths := root["paths"].(map[string]any)
	delete(paths, "/data/api/v1/tags/import")
	delete(paths, "/data/api/v1/tags/export")
	b, _ := json.Marshal(root)
	return string(b)
}

func TestBuildRecordsUnavailableScopeFromCatalog(t *testing.T) {
	in := fixtureInputsForDocument(t, withoutTagRoutes(t))
	m, err := Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if m.Qualification.Policy != reference.QualificationPolicy || slices.Contains(m.Qualification.Scopes, "tags/memory-json") || !slices.Equal(m.Qualification.UnavailableScopes, []string{"tags/memory-json"}) || len(m.Qualification.Scopes) != 5 {
		t.Fatalf("missing routes were counted as a passing round trip: %+v", m.Qualification)
	}
	_, c, err := reference.OpenCatalog(context.Background(), in.Out)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	caps, err := workflow.AssessTags(c)
	if err != nil || !testgateway.SameCapabilities(caps, m.Qualification.Capabilities) {
		t.Fatal("offline reference lost capability evidence")
	}
}

func TestBuildRejectsUnprovenTagRefusals(t *testing.T) {
	for _, mode := range []string{"missing capabilities", "old receipt", "dispatched", "missing counter", "http", "wrong error", "wrong exit", "wrong outcome", "missing refusal", "extra round trip"} {
		t.Run(mode, func(t *testing.T) {
			in := fixtureInputsForDocument(t, withoutTagRoutes(t))
			b, _ := os.ReadFile(in.Transfers)
			var receipt map[string]any
			if json.Unmarshal(b, &receipt) != nil {
				t.Fatal("invalid fixture")
			}
			switch mode {
			case "missing capabilities":
				delete(receipt, "capabilities")
			case "old receipt":
				receipt["version"] = 2
			case "extra round trip":
				receipt["checks"] = append(receipt["checks"].([]any), map[string]any{"name": "workflow-tag-import", "outcome": "completed", "operationRequests": 1})
			default:
				for _, item := range receipt["checks"].([]any) {
					check := item.(map[string]any)
					if check["name"] != "workflow-tag-import-unavailable" {
						continue
					}
					switch mode {
					case "dispatched":
						check["operationRequests"] = 1
					case "missing counter":
						delete(check, "operationRequests")
					case "http":
						check["httpStatus"] = 404
					case "wrong error":
						check["errorKind"] = "http"
					case "wrong exit":
						check["exitCode"] = 7
					case "wrong outcome":
						check["outcome"] = "completed"
					case "missing refusal":
						check["name"] = "unrelated-check"
					}
				}
			}
			b, _ = json.Marshal(receipt)
			if err := os.WriteFile(in.Transfers, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Build(context.Background(), in); err == nil {
				t.Fatal("unproven refusal evidence was qualified")
			}
			if _, err := os.Stat(in.Out); !os.IsNotExist(err) {
				t.Fatal("failed qualification published output")
			}
		})
	}
}

func TestQualificationRetainsAllHistoricalTagChecks(t *testing.T) {
	b, err := os.ReadFile("testdata/tag-checks.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Checks []struct{ Name, Outcome string }
	}
	if err := json.Unmarshal(b, &receipt); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"tag-capabilities": "completed"}
	for _, check := range receipt.Checks {
		want[check.Name] = check.Outcome
	}
	got := map[string]string{}
	for outcome, names := range requiredWorkflowChecks("project-tag-workflows", true) {
		for _, name := range names {
			got[name] = outcome
		}
	}
	if len(got) != len(want) {
		t.Fatal("full tag qualification changed scope unexpectedly")
	}
	for name, outcome := range want {
		if got[name] != outcome {
			t.Fatalf("historical check removed or weakened: %s", name)
		}
	}
}
