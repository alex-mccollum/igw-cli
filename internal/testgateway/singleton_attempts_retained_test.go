package testgateway_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedSingletonRecreationFailure(t *testing.T) {
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "singleton", "attempts", "1", path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	decode := func(path string, out any) {
		t.Helper()
		if json.Unmarshal(read(path), out) != nil {
			t.Fatal("invalid retained singleton JSON")
		}
	}
	raw := read("manifest.json")
	if inputDigest(raw) != "c2b9d0e81be97e98e6763921ffbb94bff19b263a7cf22617b29a2c81f11d7f6b" {
		t.Fatal("failed-attempt manifest changed")
	}
	var manifest struct {
		Version int
		Files   []struct {
			Path, SHA256 string
			Bytes        int64
		}
	}
	decode("manifest.json", &manifest)
	if manifest.Version != 1 || len(manifest.Files) != 18 {
		t.Fatal("incomplete attempt manifest")
	}
	seen := map[string]bool{}
	for _, f := range manifest.Files {
		if !filepath.IsLocal(f.Path) || seen[f.Path] {
			t.Fatal("invalid manifest path")
		}
		seen[f.Path] = true
		b := read(f.Path)
		if int64(len(b)) != f.Bytes || inputDigest(b) != f.SHA256 {
			t.Fatal("original attempt bytes changed")
		}
	}
	const source = "7096ce129073c85f241bf1cf5bf6c234fb969ffe"
	const binary = "5957bfcba63b2f306a9bc872615d6dfe3eca804b02c9aae22617ff87c8b2ccb9"
	var build struct {
		SourceCommit, TestBinarySHA256 string
		SourceDirty                    bool
	}
	decode("build.json", &build)
	if build.SourceCommit != source || build.TestBinarySHA256 != binary || build.SourceDirty {
		t.Fatal("failed build identity changed")
	}
	for _, path := range []string{"source-before.json", "source-after.json"} {
		var state struct {
			SourceCommit, Status string
			Clean                bool
		}
		decode(path, &state)
		if state.SourceCommit != source || state.Status != "" || !state.Clean {
			t.Fatal("source not clean and stable")
		}
	}
	var receipt inputReceipt
	decode("8.3.0/singleton/singleton.json", &receipt)
	if receipt.Passed || !receipt.Cleanup || receipt.TestBinarySHA256 != binary || len(receipt.Checks) != 16 {
		t.Fatal("failure relabeled as passing qualification")
	}
	last := receipt.Checks[len(receipt.Checks)-1]
	if last.Name != "create" || last.Outcome != "uncertain" || last.HTTPStatus != 200 || last.ExitCode != 7 || last.OperationRequests != 3 || last.Resource == nil || last.Resource.State != "changed_unverified" || last.Resource.Checks != nil {
		t.Fatal("failed recreation evidence changed")
	}
	writes := 0
	for _, wire := range last.Wire {
		if wire.Method != "GET" {
			writes++
		}
	}
	if writes != 1 {
		t.Fatal("recreation did not send exactly one mutation")
	}
	for _, stage := range []string{"lifecycle", "singleton"} {
		var process struct {
			ExitCode                                  int
			ReceiptPassed, IndependentCleanupVerified bool
		}
		decode("8.3.0/"+stage+"-process.json", &process)
		if !process.IndependentCleanupVerified || process.ReceiptPassed != (stage == "lifecycle") || (stage == "lifecycle" && process.ExitCode != 0) || (stage == "singleton" && process.ExitCode != 1) {
			t.Fatal("original process or cleanup outcome changed")
		}
		if len(read("8.3.0/after-"+stage+".txt")) != 0 {
			t.Fatal("attempt left a container")
		}
	}
	capture := read("8.3.0/singleton/openapi.json.gz")
	if receipt.OpenAPI == nil || receipt.OpenAPI.Bytes != int64(len(capture)) || receipt.OpenAPI.SHA256 != inputDigest(capture) {
		t.Fatal("original capture changed")
	}
}

func TestRetainedSingletonConfigurationReadbackGap(t *testing.T) {
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "singleton", "attempts", "2", path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	raw := read("manifest.json")
	if inputDigest(raw) != "ba13d6b362d7aa07b8b6ab0f0101ed795cdb2a5de5424d4effae238568115f79" {
		t.Fatal("original diagnostic attempt manifest changed")
	}
	var manifest struct {
		Version int
		Files   []struct {
			Path, SHA256 string
			Bytes        int64
		}
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Version != 1 || len(manifest.Files) != 18 {
		t.Fatal("invalid diagnostic manifest")
	}
	for _, file := range manifest.Files {
		if !filepath.IsLocal(file.Path) {
			t.Fatal("invalid evidence path")
		}
		b := read(file.Path)
		if int64(len(b)) != file.Bytes || inputDigest(b) != file.SHA256 {
			t.Fatal("diagnostic evidence bytes changed")
		}
	}
	var receipt inputReceipt
	if json.Unmarshal(read("8.3.0/singleton/singleton.json"), &receipt) != nil || receipt.Passed || !receipt.Cleanup || len(receipt.Checks) != 17 {
		t.Fatal("failed diagnostic attempt relabeled as passing")
	}
	creation := receipt.Checks[15]
	if creation.Name != "create" || creation.Outcome != "uncertain" || creation.ExitCode != 7 || creation.HTTPStatus != 200 || creation.Resource == nil || creation.Resource.Checks == nil {
		t.Fatal("original uncertain recreation changed")
	}
	c := creation.Resource.Checks
	if !c.Acknowledged || c.SignatureMatched == nil || !*c.SignatureMatched || c.FieldsMatched == nil || *c.FieldsMatched || len(c.MismatchedFields) != 1 || c.MismatchedFields[0] != "config" {
		t.Fatal("diagnostic cause changed")
	}
	if receipt.Checks[16].Name != "uncertain-create-readback" || receipt.Checks[16].HTTPStatus != 200 {
		t.Fatal("independent readback missing")
	}
	if len(read("8.3.0/after-singleton.txt")) != 0 {
		t.Fatal("diagnostic attempt left a container")
	}
}
