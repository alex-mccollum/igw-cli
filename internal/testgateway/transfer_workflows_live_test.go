package testgateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/project"
	"github.com/alex-mccollum/igw-cli/internal/tag"
)

func qualifyTransferWorkflows(t *testing.T, run func(string, ...string) transferResult, dir, sourceName, source string) {
	t.Helper()
	ok := func(step string, args ...string) transferResult {
		t.Helper()
		got := run(step, args...)
		if !got.OK {
			t.Fatalf("%s: %v", step, got.Error)
		}
		return got
	}
	verified := func(step string, args ...string) transferResult {
		t.Helper()
		got := ok(step, args...)
		if got.Outcome != "completed" || got.Meta.Verification != "verified" {
			t.Fatalf("%s not verified: %s", step, got.Outcome)
		}
		return got
	}
	listed := ok("workflow-project-list", "project", "list", "--limit", "50")
	if !strings.Contains(string(listed.Data), sourceName) {
		t.Fatal("project list missing source")
	}
	ok("workflow-project-get", "project", "get", sourceName)
	typedSource := filepath.Join(dir, "workflow-source.zip")
	exported := ok("workflow-project-export", "project", "export", sourceName, "--out", typedSource)
	if exported.Artifact == nil || exported.Meta.Verification != "archive_validated" || !reflect.DeepEqual(archiveContents(t, source), archiveContents(t, typedSource)) {
		t.Fatal("project export not validated")
	}
	inspected := ok("workflow-project-inspect", "project", "inspect", typedSource)
	var manifest project.Manifest
	if json.Unmarshal(inspected.Data, &manifest) != nil || len(manifest.SHA256) != 64 || len(manifest.Files) == 0 {
		t.Fatal("project manifest missing")
	}
	const copyName = "igw-workflow-copy"
	preview := ok("workflow-project-preview", "project", "import", copyName, "--in", typedSource, "--dry-run")
	if preview.Outcome != "preview" {
		t.Fatal("project preview unavailable")
	}
	absent := run("workflow-project-preview-absence", "project", "get", copyName)
	if absent.OK || absent.Error == nil {
		t.Fatal("project preview mutated Gateway")
	}
	imported := verified("workflow-project-import", "project", "import", copyName, "--in", typedSource, "--yes")
	var evidence project.ImportEvidence
	if json.Unmarshal(imported.Data, &evidence) != nil || evidence.AfterSHA256 != manifest.SHA256 {
		t.Fatal("project content differs")
	}
	ok("workflow-project-change", "api", "request", "PUT /data/api/v1/projects/{name}", "--path-param", "name="+copyName, "--body", `{"description":"changed before reviewed replacement"}`, "--yes")
	replacement := ok("workflow-project-replace-preview", "project", "import", copyName, "--in", typedSource, "--overwrite", "--dry-run")
	if json.Unmarshal(replacement.Data, &evidence) != nil || len(evidence.BeforeSHA256) != 64 || evidence.BeforeSHA256 == manifest.SHA256 || evidence.Precondition != "client_observation_only" || len(replacement.Meta.Warnings) == 0 {
		t.Fatal("replacement has no current digest or concurrency warning")
	}
	stale := run("workflow-project-stale-digest", "project", "import", copyName, "--in", typedSource, "--overwrite", "--if-project-sha256", strings.Repeat("0", 64), "--yes")
	if stale.OK || stale.Error == nil || stale.Error.Kind != "conflict" {
		t.Fatal("stale digest accepted")
	}
	verified("workflow-project-replace", "project", "import", copyName, "--in", typedSource, "--overwrite", "--if-project-sha256", evidence.BeforeSHA256, "--yes")

	const inputJSON = `{"tags":[{"name":"igwWorkflow","tagType":"Folder","tags":[{"name":"Counter","tagType":"AtomicTag","valueSource":"memory","dataType":"Int4","value":43}]}]}`
	input := filepath.Join(dir, "workflow-tags.json")
	writeInput := func(value string) {
		t.Helper()
		if err := os.WriteFile(input, []byte(strings.Replace(inputJSON, "43", value, 1)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeInput("43")
	preview = ok("workflow-tag-preview", "tag", "import", "--in", input, "--dry-run")
	if preview.Outcome != "preview" {
		t.Fatal("tag preview unavailable")
	}
	absent = run("workflow-tag-preview-absence", "api", "request", "GET /data/api/v1/tags/export", "--query", "provider=default", "--query", "type=json", "--query", "path=igwWorkflow")
	if absent.OK && hasCounterValue(absent.Data, "43") {
		t.Fatal("tag preview imported Counter")
	}
	verified("workflow-tag-import", "tag", "import", "--in", input, "--yes")
	checkTags := func(step, want string) {
		t.Helper()
		path := filepath.Join(dir, step+".json")
		got := ok(step, "tag", "export", "--provider", "default", "--path", "igwWorkflow", "--out", path)
		if got.Artifact == nil {
			t.Fatal("tag artifact missing")
		}
		raw, err := os.ReadFile(path)
		if err != nil || !hasCounterValue(raw, want) {
			t.Fatalf("%s: unexpected Counter value", step)
		}
	}
	checkTags("workflow-tag-export", "43")
	writeInput("44")
	preview = ok("workflow-tag-overwrite-preview", "tag", "import", "--in", input, "--collision-policy", "Overwrite", "--dry-run")
	if preview.Outcome != "preview" {
		t.Fatal("tag overwrite preview unavailable")
	}
	checkTags("workflow-tag-preview-unchanged", "43")
	verified("workflow-tag-overwrite", "tag", "import", "--in", input, "--collision-policy", "Overwrite", "--yes")
	checkTags("workflow-tag-overwrite-export", "44")
	writeInput("45")
	failed := run("workflow-tag-abort", "tag", "import", "--in", input, "--yes")
	var tagEvidence tag.Evidence
	if failed.OK || failed.Error == nil || failed.Error.Code != 7 || json.Unmarshal(failed.Data, &tagEvidence) != nil || tagEvidence.Report == nil || tagEvidence.Report.FailureCount == 0 {
		t.Fatal("HTTP 200 tag failures reported as success")
	}
	checkTags("workflow-tag-abort-unchanged", "44")
	ok("workflow-project-delete", "api", "request", "DELETE /data/api/v1/projects/{name}", "--path-param", "name="+copyName, "--query", "confirm=true", "--yes")
}
