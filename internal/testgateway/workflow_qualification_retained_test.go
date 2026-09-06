package testgateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedWorkflowCandidateQualification(t *testing.T) {
	root := filepath.Join("..", "..", "docs", "qualification", "workflow-v1")
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	raw := read("manifest.json")
	if inputDigest(raw) != "70a0dd99f33af69a00a9ef0684688325b87feba35d1089424a3c9a7ca20f4b5a" {
		t.Fatal("qualification manifest changed")
	}
	var manifest struct {
		Version int
		Files   []struct {
			Path, SHA256 string
			Bytes        int64
		}
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Version != 1 || len(manifest.Files) != 60 {
		t.Fatal("incomplete qualification manifest")
	}
	seen := map[string]bool{}
	for _, f := range manifest.Files {
		if !filepath.IsLocal(f.Path) || seen[f.Path] {
			t.Fatal("invalid evidence path")
		}
		seen[f.Path] = true
		b := read(f.Path)
		if int64(len(b)) != f.Bytes || inputDigest(b) != f.SHA256 {
			t.Fatal("original qualification bytes changed")
		}
	}
	var build struct {
		SourceCommit, TestBinarySHA256, CLIBinarySHA256 string
		SourceDirty                                     bool
	}
	if json.Unmarshal(read("candidate/build.json"), &build) != nil || build.SourceDirty || build.SourceCommit != "511c5fee5241f6d8157344a1ed5250b01db5edb6" {
		t.Fatal("candidate source identity changed")
	}
	for version, count := range map[string]int{"8.3.0": 48, "8.3.9": 55} {
		var r inputReceipt
		if json.Unmarshal(read("candidate/"+version+"/journeys/journeys.json"), &r) != nil || !r.Passed || !r.Cleanup || r.TestBinarySHA256 != build.TestBinarySHA256 || r.Executable == nil || r.Executable.SHA256 != build.CLIBinarySHA256 || len(r.Executable.Checks) != count {
			t.Fatal("native journey identity or outcome changed")
		}
		checks := map[string]journeyCheck{}
		for _, c := range r.Executable.Checks {
			if _, exists := checks[c.Name]; exists {
				t.Fatal("duplicate journey check")
			}
			checks[c.Name] = c
		}
		for _, name := range []string{"profile-setup", "resource-create", "resource-update", "resource-delete", "workflow-project-import", "workflow-project-replace", "logs-human", "logs-search", "logs-empty", "backup-export", "logs-download", "bundle-preview", "bundle-collect", "bundle-status"} {
			c, ok := checks[name]
			if !ok || c.ExitCode != 0 {
				t.Fatalf("required journey missing: %s", name)
			}
		}
		if checks["resource-stale-review"].ErrorKind != "conflict" || checks["workflow-project-stale-digest"].ErrorKind != "conflict" {
			t.Fatal("reviewed preconditions not qualified")
		}
		if version == "8.3.0" {
			for _, name := range []string{"workflow-tag-import-unavailable", "workflow-tag-preview-unavailable", "workflow-tag-export-unavailable"} {
				c := checks[name]
				if c.ExitCode != 2 || c.ErrorKind != "capability" {
					t.Fatal("absent tag API relabeled as success")
				}
			}
		} else {
			for _, name := range []string{"workflow-tag-import", "workflow-tag-overwrite"} {
				if checks[name].Verification != "verified" {
					t.Fatal("tag transfer not verified")
				}
			}
			if checks["workflow-tag-abort"].ExitCode != 7 {
				t.Fatal("tag failures reported as success")
			}
		}
		for _, stage := range []string{"lifecycle", "journeys"} {
			var p struct {
				ExitCode                                  int
				ReceiptPassed, IndependentCleanupVerified bool
			}
			if json.Unmarshal(read("candidate/"+version+"/"+stage+"-process.json"), &p) != nil || p.ExitCode != 0 || !p.ReceiptPassed || !p.IndependentCleanupVerified || len(read("candidate/"+version+"/after-"+stage+".txt")) != 0 {
				t.Fatal("live cleanup or process outcome changed")
			}
		}
	}
	for _, path := range []string{"singleton-8.3.0/8.3.0/singleton/singleton.json", "candidate/8.3.9/singleton/singleton.json"} {
		var r inputReceipt
		if json.Unmarshal(read(path), &r) != nil || !r.Passed || !r.Cleanup {
			t.Fatal("singleton limited acceptance failed")
		}
		checks := map[string]inputCheck{}
		for _, c := range r.Checks {
			checks[c.Name] = c
		}
		if checks["create-with-config"].Outcome != "uncertain" || checks["metadata-create"].ErrorKind != "resource_rejected" || checks["metadata-rejection-readback"].HTTPStatus != 404 {
			t.Fatal("singleton limitations relabeled")
		}
	}
}
