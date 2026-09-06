package testgateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedRestartBaselineFailures(t *testing.T) {
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "restart", "attempts", path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	decode := func(path string, out any) {
		t.Helper()
		if json.Unmarshal(read(path), out) != nil {
			t.Fatal("invalid retained attempt JSON")
		}
	}
	raw := read("manifest.json")
	if inputDigest(raw) != "57e1ed3b5805efefb211e1d8fa4747a9a79fb8448d3d83c6d9dd163b76f47409" {
		t.Fatal("failed-attempt integrity manifest changed")
	}
	var manifest struct {
		Version int
		Files   []struct {
			Path, SHA256 string
			Bytes        int64
		}
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Version != 1 || len(manifest.Files) != 26 {
		t.Fatal("failed-attempt manifest is incomplete")
	}
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if !filepath.IsLocal(file.Path) || seen[file.Path] {
			t.Fatal("invalid manifest path")
		}
		seen[file.Path] = true
		b := read(file.Path)
		if int64(len(b)) != file.Bytes || inputDigest(b) != file.SHA256 {
			t.Fatal("failed-attempt artifact identity changed")
		}
	}
	for _, expected := range []struct{ id, source, binary string }{
		{"1", "3ac2eed58c9e665b4dfefce7c068f3eb9f32dc9e", "de1dcd40bc18ff3d39b0fc04c7867a4072f39df9320029c60c60171d1018a9c1"},
		{"2", "1920701bc843a502145f4253752e8e97853cbbfd", "a9b016425ecf61070f81ea952fb59a5a517026e49f88c5bd7b2a28713ef9c3b6"},
	} {
		prefix := expected.id + "/"
		var build struct {
			SourceCommit, TestBinarySHA256 string
			SourceDirty                    bool
		}
		decode(prefix+"build.json", &build)
		if build.SourceCommit != expected.source || build.TestBinarySHA256 != expected.binary || build.SourceDirty {
			t.Fatal("failed-attempt build changed")
		}
		for _, path := range []string{"source-before.json", "source-after.json"} {
			var source struct {
				Commit string
				Dirty  bool
			}
			decode(prefix+path, &source)
			if source.Commit != expected.source || source.Dirty {
				t.Fatal("failed-attempt source was not clean and stable")
			}
		}
		var receipt inputReceipt
		decode(prefix+"8.3.9/restart/restart.json", &receipt)
		var process struct {
			LifecycleExitCode, RestartExitCode int
			IndependentCleanupVerified         bool
		}
		decode(prefix+"8.3.9/process.json", &process)
		var lifecycle struct {
			Passed, Cleanup  bool
			TestBinarySHA256 string
			Checks           []string
		}
		decode(prefix+"8.3.9/lifecycle.json", &lifecycle)
		if receipt.Passed || !receipt.Cleanup || receipt.TestBinarySHA256 != expected.binary || len(receipt.Checks) != 5 || process.LifecycleExitCode != 0 || process.RestartExitCode != 1 || !process.IndependentCleanupVerified || !lifecycle.Passed || !lifecycle.Cleanup || lifecycle.TestBinarySHA256 != expected.binary || len(lifecycle.Checks) != 10 {
			t.Fatal("failed baseline was misrepresented as successful qualification")
		}
		for index, check := range receipt.Checks {
			if check.Restart != nil && check.Restart.ConfirmedRequests != 0 {
				t.Fatal("failed baseline sent restart")
			}
			for _, wire := range check.Wire {
				if wire.Method != "GET" {
					t.Fatal("failed baseline mutated Gateway")
				}
			}
			if index < 4 && check.OperationRequests != 0 {
				t.Fatal("baseline preflight sent operations")
			}
		}
		last := receipt.Checks[4]
		if last.Name != "preview" || last.Outcome != "preview" || last.OperationRequests != 4 || last.Restart == nil || last.Restart.Polls != 0 || last.Restart.Acknowledged || last.Process != nil {
			t.Fatal("failed baseline acquired or invented Java evidence")
		}
		for _, path := range []string{"preflight-containers.txt", "8.3.9/after-lifecycle.txt", "8.3.9/after-restart.txt"} {
			if len(read(prefix+path)) != 0 {
				t.Fatal("failed attempt left a container")
			}
		}
		if !strings.HasSuffix(string(read(prefix+"8.3.9/restart.log")), "FAIL\n") {
			t.Fatal("failed process log missing")
		}
		capture := read(prefix + "8.3.9/restart/openapi.json.gz")
		if receipt.OpenAPI == nil || receipt.OpenAPI.Bytes != int64(len(capture)) || receipt.OpenAPI.SHA256 != inputDigest(capture) {
			t.Fatal("failed-attempt capture changed")
		}
	}
}
