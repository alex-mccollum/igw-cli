package tag

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

type requiringRunner struct {
	t      *testing.T
	wantID string
	checks int
}

func (r *requiringRunner) Run(execute.Request) result.Result {
	r.t.Fatal("workflow dispatched after a missing capability")
	return result.Result{}
}
func (r *requiringRunner) Require(requirement catalog.Capability) result.Result {
	r.checks++
	if requirement.ID != r.wantID {
		r.t.Fatalf("wrong requirement: %s", requirement.ID)
	}
	return result.Failure(&result.Problem{Kind: "capability", Code: 2})
}

func TestTagPrerequisitesMatchVerificationMode(t *testing.T) {
	for _, format := range []string{"json", "xml", "csv"} {
		for _, policy := range []string{"Abort", "Overwrite", "MergeOverwrite", "Rename", "Ignore"} {
			for _, preview := range []bool{false, true} {
				id := "tag.import"
				if format == "json" && policy != "Rename" && policy != "Ignore" {
					id = "tag.import.verified-json"
				}
				runner := &requiringRunner{t: t, wantID: id}
				got := Import(context.Background(), runner, ImportRequest{Provider: "default", Format: format, CollisionPolicy: policy, Source: tagSource(t, tree), Yes: !preview, DryRun: preview})
				if got.OK || got.Error.Kind != "capability" || runner.checks != 1 {
					t.Fatalf("missing capability was bypassed: %s %s preview=%v", format, policy, preview)
				}
			}
		}
	}
	runner := &requiringRunner{t: t, wantID: "tag.export"}
	got := Export(runner, ExportRequest{Provider: "default", Format: "json", Out: "unused.json"})
	if got.OK || got.Error.Kind != "capability" || runner.checks != 1 {
		t.Fatal("export bypassed capability check")
	}
}

func TestCapturedTagCapabilities(t *testing.T) {
	// Parser evidence from actual vendor documents, not live-workflow acceptance.
	for _, version := range []string{"8.3.0", "8.3.9"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join("..", "reference", "bundles", "ignition-"+version+"-defaults", "openapi.json.gz")
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			reader, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			raw, err := io.ReadAll(io.LimitReader(reader, catalog.MaxDocumentBytes+1))
			if err != nil {
				t.Fatal(err)
			}
			c, err := catalog.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			for _, requirement := range workflow.TagRequirements() {
				got, err := c.Assess(requirement)
				if err != nil || (got.Status == "advertised") != (version == "8.3.9") {
					t.Fatalf("incorrect captured tag capability: %+v %v", got, err)
				}
				if version == "8.3.0" && len(got.MissingOperations) != len(requirement.RequiredOperations) {
					t.Fatal("minimum catalog unexpectedly advertises a tag transfer route")
				}
			}
		})
	}
}
