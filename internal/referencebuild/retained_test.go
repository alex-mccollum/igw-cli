package referencebuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/imageref"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

// Original live receipts, rather than generated fixtures, exercise cross-file
// qualification. Rechecking them does not renew their observation timestamps.
func TestRetainedMinimumCapabilityQualification(t *testing.T) {
	verifyRetainedQualification(t, "ignition-8.3.0-policy2", 672, "30d6a91031f2495477e53c38bfdec7dfebb0157820a661c7e6a387c839b5f800", false)
}

func TestRetainedFullTagCapabilityQualification(t *testing.T) {
	verifyRetainedQualification(t, "ignition-8.3.9-policy2", 687, "17b4ace179c02c79f4af74485773cb5afc4c35b179f7fe020c4326cb0dacceea", true)
}

func verifyRetainedQualification(t *testing.T, name string, operations int, contract string, tags bool) {
	t.Helper()
	ctx := context.Background()
	dir := filepath.Join("testdata", name, "reference")
	m, c, err := reference.OpenCatalog(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.OperationCount() != operations || len(m.Modules) != 32 || c.ContractHash() != contract {
		t.Fatal("retained catalog identity changed")
	}
	scopes := 6
	var unavailable []string
	if !tags {
		scopes = 5
		unavailable = []string{"tags/memory-json"}
	}
	if m.Qualification.Policy != reference.QualificationPolicy || len(m.Qualification.Scopes) != scopes || !slices.Equal(m.Qualification.UnavailableScopes, unavailable) || slices.Contains(m.Qualification.Scopes, "tags/memory-json") != tags {
		t.Fatal("qualified tag scope differs from its observed capability")
	}
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	image := imageref.Candidate{Index: read("registry-index.json"), Manifest: read("registry-manifest.json")}
	if err := json.Unmarshal(read("resolution.json"), &image.Resolution); err != nil {
		t.Fatal(err)
	}
	imageID, err := image.ConfigurationDigest()
	if err != nil {
		t.Fatal(err)
	}
	var capture testgateway.Evidence
	if err := json.Unmarshal(read("capture.json"), &capture); err != nil {
		t.Fatal(err)
	}
	if err := validateCapture(capture, image.Resolution, imageID); err != nil {
		t.Fatal(err)
	}
	if c.RawHash() != capture.RawSHA256 || c.DocumentHash() != capture.DocumentSHA256 || c.ContractHash() != capture.ContractSHA256 {
		t.Fatal("capture receipt differs from retained vendor bytes")
	}
	if err := validateLifecycle(read("lifecycle.json"), capture, m.Qualification.TestBinarySHA256); err != nil {
		t.Fatal(err)
	}
	capabilities, err := workflow.AssessTags(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"resource-workflows", "project-tag-workflows", "operational-workflows"} {
		if err := validateWorkflow(read(kind+".json"), kind, capture, m.Qualification.TestBinarySHA256, capabilities); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}
