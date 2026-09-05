package testgateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/operations"
)

func qualifyOperationalWorkflows(t *testing.T, run func(string, ...string) transferResult, dir string) []operationalArtifact {
	t.Helper()
	listed := run("workflow-logs-list", "logs", "list", "--limit", "5", "--min-level", "INFO", "--since", "1h")
	var list struct {
		Items    []json.RawMessage
		Metadata struct{ Limit int }
	}
	if json.Unmarshal(listed.Data, &list) != nil || list.Metadata.Limit != 5 || len(list.Items) > 5 {
		t.Fatal("log filter or pagination not applied")
	}
	var artifacts []operationalArtifact
	download := func(step, filename string, args ...string) transferResult {
		t.Helper()
		path := filepath.Join(dir, filename)
		got := run(step, append(args, "--out", path, "--max-bytes", "134217728")...)
		if got.Artifact == nil {
			t.Fatal("operational artifact missing")
		}
		detail := inspectOperationalArtifact(t, step, path)
		if detail.SHA256 != got.Artifact.SHA256 || detail.Bytes != got.Artifact.Bytes {
			t.Fatal("artifact receipt differs")
		}
		artifacts = append(artifacts, detail)
		return got
	}
	download("workflow-backup-export", "workflow.gwbk", "backup", "export")
	download("workflow-logs-download", "workflow-logs.idb", "logs", "download")
	before := run("workflow-bundle-status", "diagnostics", "bundle", "status")
	var status operations.BundleStatus
	if json.Unmarshal(before.Data, &status) != nil || status.State != "ready" || status.FileSize <= 0 {
		t.Fatal("ready bundle not recognized")
	}
	previewPath := filepath.Join(dir, "preview-not-created.zip")
	preview := run("workflow-bundle-preview", "diagnostics", "bundle", "collect", "--out", previewPath, "--dry-run")
	if preview.Outcome != "preview" {
		t.Fatal("collection preview did not remain preview")
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Fatal("preview created an artifact")
	}
	after := run("workflow-bundle-preview-unchanged", "diagnostics", "bundle", "status")
	var unchanged operations.BundleStatus
	if json.Unmarshal(after.Data, &unchanged) != nil || unchanged != status {
		t.Fatal("collection preview changed bundle")
	}
	collected := download("workflow-bundle-collect", "workflow-bundle.zip", "diagnostics", "bundle", "collect", "--yes", "--interval", "100ms")
	var evidence operations.BundleEvidence
	if json.Unmarshal(collected.Data, &evidence) != nil || !evidence.GenerationAcknowledged || evidence.Polls < 1 || evidence.Last.State != "ready" || evidence.Last.FileSize != collected.Artifact.Bytes || evidence.Correlation != "gateway_latest" || collected.Meta.Verification != "size_matched" || len(collected.Meta.Warnings) == 0 {
		t.Fatal("bundle collection did not establish its declared evidence")
	}
	again := download("workflow-bundle-download", "workflow-bundle-again.zip", "diagnostics", "bundle", "download")
	if again.Artifact.SHA256 != collected.Artifact.SHA256 || again.Meta.Verification != "size_matched" {
		t.Fatal("latest bundle changed without generation")
	}
	return artifacts
}
