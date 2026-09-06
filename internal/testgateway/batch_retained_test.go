package testgateway_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

func TestRetainedBatchEvidence(t *testing.T) {
	const source = "2ddc7f4a77e7a59b38b070fe8b6059ac322f0190"
	const binary = "3955930c5a6efe029afa1bd3166657908dc452f744cdbfa1becdda0826f5809c"
	read := func(t *testing.T, path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "batch", path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	decode := func(t *testing.T, path string, out any) {
		t.Helper()
		if err := json.Unmarshal(read(t, path), out); err != nil {
			t.Fatal(err)
		}
	}
	checkInputEvidenceManifest(t, read(t, "manifest.json"), "3bb3bc0b125e487959122506d04b668da18c46f8ab8b5c99866a309b871e599a", read)
	var before, after struct {
		Commit     string
		Dirty      bool
		ObservedAt time.Time
	}
	decode(t, "source-before.json", &before)
	decode(t, "source-after.json", &after)
	var build struct {
		SourceCommit, TestBinarySHA256, GoVersion string
		SourceDirty, Trimpath                     bool
		FinishedAt                                time.Time
	}
	decode(t, "build.json", &build)
	if before.Commit != source || after.Commit != source || before.Dirty || after.Dirty || build.SourceCommit != source || build.SourceDirty || build.TestBinarySHA256 != binary || build.GoVersion != "go1.27.1" || !build.Trimpath || build.FinishedAt.Before(before.ObservedAt) || !after.ObservedAt.After(build.FinishedAt) {
		t.Fatal("clean-source build provenance changed")
	}
	if len(read(t, "preflight-containers.txt")) != 0 {
		t.Fatal("qualification started with a leftover container")
	}
	for _, version := range []string{"8.3.0", "8.3.9"} {
		t.Run(version, func(t *testing.T) {
			var receipt inputReceipt
			decode(t, version+"/batch/batch.json", &receipt)
			var lifecycle struct {
				Kind, Image, ImageID, Platform, TestBinarySHA256 string
				Checks                                           []string
				StartedAt, FinishedAt                            time.Time
				LifetimeSeconds, ExitCode                        int
				Passed, Cleanup                                  bool
			}
			decode(t, version+"/lifecycle.json", &lifecycle)
			var process struct {
				LifecycleExitCode, BatchExitCode int
				IndependentCleanupVerified       bool
			}
			decode(t, version+"/process.json", &process)
			if receipt.Version != 1 || receipt.Kind != "batch-workflows" || !receipt.Passed || !receipt.Cleanup || !lifecycle.Passed || !lifecycle.Cleanup || process.LifecycleExitCode != 0 || process.BatchExitCode != 0 || !process.IndependentCleanupVerified {
				t.Fatal("incomplete live run")
			}
			if receipt.TestBinarySHA256 != binary || lifecycle.TestBinarySHA256 != binary || lifecycle.Image != receipt.Image || lifecycle.ImageID != receipt.ImageID || lifecycle.Platform != receipt.Platform || receipt.Platform != "linux/amd64" || !strings.HasPrefix(receipt.GatewayVersion, version+" ") {
				t.Fatal("executable or image identities differ")
			}
			if lifecycle.Kind != "capture-lifecycle" || len(lifecycle.Checks) != 10 || lifecycle.LifetimeSeconds != 5 || lifecycle.ExitCode != 124 || lifecycle.StartedAt.Before(build.FinishedAt) || receipt.StartedAt.Before(lifecycle.FinishedAt) || !receipt.FinishedAt.After(receipt.StartedAt) || after.ObservedAt.Before(receipt.FinishedAt) {
				t.Fatal("lifecycle or observation chronology changed")
			}
			profile, _ := moduleprofile.Select("core-opcua")
			if err := profile.ValidateInventory(receipt.ModuleInventory); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"after-lifecycle.txt", "after-batch.txt"} {
				if len(read(t, version+"/"+path)) != 0 {
					t.Fatal("independent cleanup query found a container")
				}
			}
			for _, path := range []string{"lifecycle.log", "batch.log"} {
				if !strings.HasSuffix(string(read(t, version+"/"+path)), "PASS\n") {
					t.Fatal("live process did not pass")
				}
			}
			checkRetainedBatchChecks(t, receipt.Checks)
			compressed := read(t, version+"/batch/openapi.json.gz")
			if receipt.OpenAPI == nil || receipt.Catalog == nil || receipt.OpenAPI.Bytes != int64(len(compressed)) || receipt.OpenAPI.SHA256 != inputDigest(compressed) {
				t.Fatal("vendor artifact identity changed")
			}
			gz, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(io.LimitReader(gz, catalog.MaxDocumentBytes+1))
			closeErr := gz.Close()
			if err != nil || closeErr != nil || len(raw) > catalog.MaxDocumentBytes {
				t.Fatal("cannot read vendor capture")
			}
			c, err := catalog.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			m := receipt.Catalog
			// The receipt keeps its historical parser. Reopening these exact
			// bytes with today's parser does not renew live qualification.
			if m.ParserVersion != "libopenapi/0.38.7+validator/0.14.0;igw/19" || m.SourceKind != "gateway" || m.ContractPolicy != "igw-contract/2" || m.RawSHA256 != c.RawHash() || m.DocumentSHA256 != c.DocumentHash() || m.ContractSHA256 != c.ContractHash() || m.FetchedAt.Before(receipt.StartedAt) || m.VerifiedAt.After(receipt.FinishedAt) {
				t.Fatal("catalog provenance changed")
			}
		})
	}
}

func checkRetainedBatchChecks(t *testing.T, checks []inputCheck) {
	t.Helper()
	want := []struct {
		name, outcome                     string
		code, operations, catalogRequests int
		items                             string
	}{
		{"catalog", "completed", 0, 0, 1, ""},
		{"malformed-late-field", "failed", 2, 0, 0, ""},
		{"duplicate-id", "failed", 2, 0, 0, ""},
		{"confirmation", "failed", 2, 0, 0, "before:not_run encrypt:not_run after:not_run"},
		{"preview", "preview", 0, 0, 0, "before:preview encrypt:preview after:preview"},
		{"offline-preview", "preview", 0, 0, 0, "before:preview encrypt:preview after:preview"},
		{"read-only", "completed", 0, 2, 0, "before:completed after:completed"},
		{"complete", "accepted", 0, 3, 1, "before:completed encrypt:accepted after:completed"},
		{"stop-validation", "partial", 2, 1, 1, "encrypt:accepted invalid:failed after:not_run"},
		{"continue-validation", "partial", 2, 2, 1, "encrypt:accepted invalid:failed after:completed"},
		{"stop-http", "partial", 7, 2, 0, "before:completed missing:failed after:not_run"},
		{"continue-http", "partial", 7, 3, 0, "before:completed missing:failed after:completed"},
		{"continue-unknown-operation", "partial", 2, 2, 0, "before:completed unknown:failed after:completed"},
	}
	if len(checks) != len(want) {
		t.Fatal("missing batch checks")
	}
	text := []byte("igw batch public fixture + & 日本")
	for n, expected := range want {
		check := checks[n]
		if check.Name != expected.name || check.Outcome != expected.outcome || check.ExitCode != expected.code || check.OperationRequests != int64(expected.operations) || len(check.Wire) != expected.operations || check.CatalogRequests != expected.catalogRequests {
			t.Fatalf("batch check changed: %s", expected.name)
		}
		for _, wire := range check.Wire {
			body := []byte(nil)
			if wire.Method == "POST" {
				body = text
				if wire.Path != "/data/api/v1/encryption/encrypt" || wire.ContentType != "text/plain" {
					t.Fatal("batch write target changed")
				}
			} else if wire.Method != "GET" {
				t.Fatal("unexpected batch mutation")
			}
			if wire.BodyBytes != int64(len(body)) || wire.ContentLength != wire.BodyBytes || wire.BodySHA256 != inputDigest(body) {
				t.Fatal("consumed batch payload identity changed")
			}
		}
		items := strings.Fields(expected.items)
		if len(items) == 0 {
			if check.Batch != nil {
				t.Fatal("unexpected batch report")
			}
			continue
		}
		if check.Batch == nil || len(check.Batch.Items) != len(items) {
			t.Fatal("missing ordered item outcomes")
		}
		passed, failed, notRun := 0, 0, 0
		for j, entry := range items {
			id, outcome, _ := strings.Cut(entry, ":")
			item := check.Batch.Items[j]
			if item.ID != id || item.Outcome != outcome {
				t.Fatal("item order or outcome changed")
			}
			switch outcome {
			case "not_run":
				notRun++
				if item.HTTPStatus != 0 || item.ErrorKind != "" || item.ExitCode != 0 {
					t.Fatal("unattempted item has fabricated status")
				}
			case "failed":
				failed++
				status, code, kind := 0, 2, "usage"
				if id == "invalid" {
					kind = "validation"
				}
				if id == "missing" {
					status, code, kind = 404, 7, "http"
				}
				if item.HTTPStatus != status || item.ExitCode != code || item.ErrorKind != kind {
					t.Fatal("item error identity changed")
				}
			case "preview":
				passed++
				if item.Preview == nil || item.HTTPStatus != 0 || item.ExitCode != 0 {
					t.Fatal("missing non-dispatched preview")
				}
				if id == "encrypt" && (!item.Preview.BodyPresent || item.Preview.BodySHA256 != inputDigest(text) || item.Preview.BodyBytes != int64(len(text))) {
					t.Fatal("prepared text identity changed")
				}
			default:
				passed++
				if item.HTTPStatus != 200 || item.ExitCode != 0 || item.Validation != "declared_schema" {
					t.Fatal("successful result lost validation/status")
				}
			}
		}
		if check.Batch.Succeeded != passed || check.Batch.Failed != failed || check.Batch.NotRun != notRun {
			t.Fatal("item counts changed")
		}
	}
}
