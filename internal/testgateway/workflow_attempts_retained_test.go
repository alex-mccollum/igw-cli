package testgateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedWorkflowFailures(t *testing.T) {
	for _, tc := range []struct{ kind, attempt, digest string }{
		{"singleton", "3", "0e7260f1dfa6ff42daf5be011b6b07fb725575627d179e36b01144bab018a8a0"},
		{"journeys", "1", "c813d6e63c15a62879c6eae606d899a4a1121d2df14f06d1d0e0439cb10f2830"},
		{"journeys", "2", "7c7d666897abd5d72337c7d6cf566788eef00c4df30929a4eb77b6ad3f8cfed8"},
		{"journeys", "3", "7c07270e5d5357762e56fa4ac7f512eb4a8cbb88718fc6f432e718cb73b9c25e"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			root := filepath.Join("testdata", tc.kind, "attempts", tc.attempt)
			read := func(path string) []byte {
				t.Helper()
				b, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			raw := read("manifest.json")
			if inputDigest(raw) != tc.digest {
				t.Fatal("original manifest changed")
			}
			var m struct {
				Version int
				Files   []struct {
					Path, SHA256 string
					Bytes        int64
				}
			}
			if json.Unmarshal(raw, &m) != nil || m.Version != 1 || len(m.Files) != 18 {
				t.Fatal("incomplete manifest")
			}
			seen := map[string]bool{}
			for _, f := range m.Files {
				if !filepath.IsLocal(f.Path) || seen[f.Path] {
					t.Fatal("invalid evidence path")
				}
				seen[f.Path] = true
				b := read(f.Path)
				if int64(len(b)) != f.Bytes || inputDigest(b) != f.SHA256 {
					t.Fatal("original evidence changed")
				}
			}
			binary := "21dbce8483678dada5c425ead66d2357a4987cfe060e317988b0da4bbad33cee"
			if tc.kind == "journeys" && tc.attempt == "2" {
				binary = "65ce3716f6e3336da766f764521c2f399f913b5a05f0cd80b56a78e20531ab30"
			}
			if tc.kind == "journeys" && tc.attempt == "3" {
				binary = "558561efe1c418afb56200ed25af582244e64f1a6b44cdd92ecc0641481d59ab"
			}
			var r inputReceipt
			if json.Unmarshal(read("8.3.0/"+tc.kind+"/"+tc.kind+".json"), &r) != nil || r.Passed || !r.Cleanup || r.TestBinarySHA256 != binary {
				t.Fatal("failed receipt relabeled")
			}
			if len(read("8.3.0/after-"+tc.kind+".txt")) != 0 {
				t.Fatal("cleanup missing")
			}
			if tc.kind == "singleton" {
				if len(r.Checks) != 21 {
					t.Fatal("missing singleton checks")
				}
				last := r.Checks[20]
				if last.Name != "metadata-create" || last.ErrorKind != "resource_rejected" || last.ExitCode != 7 || last.Resource == nil || last.Resource.State != "unchanged" {
					t.Fatal("rejection changed")
				}
			} else if tc.attempt == "3" {
				if r.Executable == nil || len(r.Executable.Checks) != 45 || r.Executable.Checks[44].Name != "bundle-preview" || r.Executable.Checks[44].ErrorKind != "response" || r.Executable.Checks[44].ExitCode != 7 {
					t.Fatal("diagnostics preview failure changed")
				}
			} else if tc.attempt == "2" {
				if r.Executable == nil || len(r.Executable.Checks) != 20 || r.Executable.Checks[19].Name != "capabilities" || r.Executable.Checks[19].ExitCode != 0 {
					t.Fatal("capability selection failure changed")
				}
			} else {
				if r.Executable == nil || len(r.Executable.Checks) != 1 || r.Executable.Checks[0].Name != "offline-help" || r.Executable.Checks[0].ExitCode != 2 {
					t.Fatal("help failure changed")
				}
			}
		})
	}
}
