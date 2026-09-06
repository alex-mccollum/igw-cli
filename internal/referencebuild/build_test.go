package referencebuild

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/imageref"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

const syntheticDocument = `{"openapi":"3.1.0","info":{"title":"Synthetic reference fixture","version":"1.0.0"},"paths":{"/health":{"get":{"responses":{"200":{"description":"OK"}}}},"/data/api/v1/tags/import":{"post":{"responses":{"200":{"description":"OK"}}}},"/data/api/v1/tags/export":{"get":{"responses":{"200":{"description":"OK"}}}}}}`

func TestBuildRejectsHistoricalParserEvidence(t *testing.T) {
	in := fixtureInputs(t)
	const recordedParser = "libopenapi/0.38.7+validator/0.14.0;igw/12"
	if recordedParser == catalog.ParserVersion {
		t.Fatal("test requires a prior parser identity")
	}
	mutateReceipt(t, filepath.Join(in.CaptureDir, "capture.json"), func(c map[string]any) {
		c["parserVersion"] = recordedParser
	})
	for _, path := range []string{in.Resources, in.Transfers, in.Operations} {
		mutateReceipt(t, path, func(r map[string]any) {
			r["catalog"].(map[string]any)["parserVersion"] = recordedParser
		})
	}
	if _, err := Build(context.Background(), in); err == nil {
		t.Fatal("consistent historical receipts were promoted to current qualification")
	}
	if _, err := os.Stat(in.Out); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale qualification created an output directory")
	}
}

// Synthetic receipts exercise cross-file validation; only the opt-in Gateway
// runs provide runtime qualification evidence.
func fixtureInputs(t *testing.T) Inputs {
	t.Helper()
	return fixtureInputsForDocument(t, syntheticDocument)
}

func fixtureInputsForDocument(t *testing.T, document string) Inputs {
	t.Helper()
	dir := t.TempDir()
	in := Inputs{ResolutionDir: filepath.Join(dir, "resolution"), CaptureDir: filepath.Join(dir, "capture"), Lifecycle: filepath.Join(dir, "lifecycle.json"), Resources: filepath.Join(dir, "resources.json"), Transfers: filepath.Join(dir, "transfers.json"), Operations: filepath.Join(dir, "operations.json"), TestBinary: filepath.Join(dir, "test-binary"), Baseline: filepath.Join(dir, "baseline.json"), Out: filepath.Join(dir, "bundle")}
	write := func(path string, v any) {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	configDigest := "sha256:" + strings.Repeat("a", 64)
	body, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": map[string]any{"digest": configDigest, "size": 10}, "layers": []any{map[string]any{"digest": configDigest, "size": 20}}})
	index, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": []any{map[string]any{"mediaType": "application/vnd.oci.image.manifest.v1+json", "digest": "sha256:" + digest(body), "size": len(body), "platform": map[string]any{"os": "linux", "architecture": "amd64"}}}})
	r := imageref.Resolution{Version: 1, Repository: imageref.Repository, Tag: "8.3.9", ResolvedAt: now.Add(-3 * time.Minute), Image: imageref.Repository + "@sha256:" + digest(index), IndexDigest: "sha256:" + digest(index), ManifestDigest: "sha256:" + digest(body), Platform: imageref.Platform}
	if err := (imageref.Candidate{Resolution: r, Index: index, Manifest: body}).Save(context.Background(), in.ResolutionDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(in.CaptureDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in.CaptureDir, "openapi.json"), []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in.Baseline, []byte(strings.Replace(document, "OK", "Earlier description", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in.TestBinary, []byte("synthetic test binary"), 0600); err != nil {
		t.Fatal(err)
	}
	binaryHash := digest([]byte("synthetic test binary"))
	flag := false
	modules := []testgateway.Module{{ID: "com.inductiveautomation.opcua", Version: "10.3.9", Collection: "healthy", State: "ACTIVE", OnStartup: "enabled", ShouldUpgrade: &flag}}
	moduleBytes, _ := json.Marshal(modules)
	inventory := &testgateway.ModuleInventory{Version: 1, ObservedAt: now.Add(-30 * time.Second), SHA256: digest(append([]byte("igw-module-inventory/1\n"), moduleBytes...)), Modules: modules}
	c, err := catalog.Parse([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	capture := testgateway.Evidence{Version: 3, Image: r.Image, ImageID: configDigest, Platform: r.Platform, GatewayVersion: "8.3.9 (b2026082511)", ModuleInventory: inventory, Source: "/openapi.json", CapturedAt: now.Add(-20 * time.Second), RawSHA256: c.RawHash(), DocumentSHA256: c.DocumentHash(), ContractSHA256: c.ContractHash(), ContractPolicy: catalog.ContractPolicy, ParserVersion: catalog.ParserVersion, Operations: c.OperationCount(), Validated: true, Cleanup: true}
	write(filepath.Join(in.CaptureDir, "capture.json"), capture)
	life := map[string]any{"version": 1, "kind": "capture-lifecycle", "image": r.Image, "imageId": configDigest, "platform": r.Platform, "testBinarySha256": binaryHash, "startedAt": now.Add(-2 * time.Minute), "finishedAt": now.Add(-110 * time.Second), "lifetimeSeconds": 5, "elapsedSeconds": 6, "exitCode": 124, "cleanup": true, "passed": true, "checks": []string{"image-platform", "container-image-identity", "configured-limits", "kernel-limits", "loopback-only", "exclusive-admission", "lifetime-termination", "no-oom", "owned-cleanup", "absence-after-cleanup"}}
	write(in.Lifecycle, life)
	capabilities, err := workflow.AssessTags(c)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := testgateway.TagRoundTripAvailable(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	for kind, path := range map[string]string{"resource-workflows": in.Resources, "project-tag-workflows": in.Transfers, "operational-workflows": in.Operations} {
		checks := []map[string]any{}
		for outcome, names := range requiredWorkflowChecks(kind, tags) {
			for _, name := range names {
				check := map[string]any{"name": name, "outcome": outcome, "operationRequests": 1}
				for _, refused := range unavailableTagChecks {
					if name == refused {
						check["operationRequests"], check["errorKind"], check["exitCode"] = 0, "capability", 2
					}
				}
				checks = append(checks, check)
			}
		}
		meta := catalog.Metadata{Version: catalog.SnapshotVersion, Target: catalog.Target{URL: "http://127.0.0.1:12345"}, Source: "http://127.0.0.1:12345/openapi.json", SourceKind: "gateway", FetchedAt: now.Add(-25 * time.Second), VerifiedAt: now.Add(-24 * time.Second), RawSHA256: c.RawHash(), DocumentSHA256: c.DocumentHash(), ContractSHA256: c.ContractHash(), ContractPolicy: catalog.ContractPolicy, ParserVersion: catalog.ParserVersion}
		receipt := map[string]any{"version": 2, "kind": kind, "image": r.Image, "imageId": configDigest, "platform": r.Platform, "gatewayVersion": capture.GatewayVersion, "testBinarySha256": binaryHash, "startedAt": now.Add(-time.Minute), "finishedAt": now.Add(-10 * time.Second), "moduleInventory": inventory, "catalog": meta, "checks": checks, "cleanup": true, "passed": true}
		if kind == "project-tag-workflows" {
			receipt["version"], receipt["capabilities"] = 3, capabilities
		}
		write(path, receipt)
	}
	return in
}

func TestBuildPreservesEvidenceAndLoadsOffline(t *testing.T) {
	in := fixtureInputs(t)
	m, err := Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 1 || m.Qualification.Evidence.URI != "evidence/qualification.json" || len(m.Modules) != 1 {
		t.Fatalf("incorrect qualification: %+v", m)
	}
	evidenceRaw, err := os.ReadFile(filepath.Join(in.Out, "evidence", "qualification.json"))
	var evidence struct {
		Comparison catalog.Comparison `json:"comparison"`
		Files      []reference.File   `json:"files"`
	}
	if err != nil || json.Unmarshal(evidenceRaw, &evidence) != nil || digest(evidenceRaw) != m.Qualification.Evidence.SHA256 || !evidence.Comparison.ContractEqual || evidence.Comparison.DocumentEqual || len(evidence.Files) != 8 {
		t.Fatal("runtime summary lost its audit packet or comparison")
	}
	loaded, c, err := reference.OpenCatalog(context.Background(), in.Out)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if loaded.Name != m.Name || string(c.Raw()) != syntheticDocument || c.Identity() != m.Catalog {
		t.Fatal("reference lost the exact vendor contract")
	}
	for _, pair := range [][2]string{{in.Resources, "resource-workflows.json"}, {in.Lifecycle, "lifecycle.json"}, {filepath.Join(in.CaptureDir, "capture.json"), "capture.json"}} {
		a, _ := os.ReadFile(pair[0])
		b, _ := os.ReadFile(filepath.Join(in.Out, "evidence", pair[1]))
		if string(a) != string(b) {
			t.Fatal("original evidence changed")
		}
	}
	prior, _ := os.ReadFile(filepath.Join(in.Out, "reference.json"))
	if _, err := Build(context.Background(), in); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing reference replaced: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(in.Out, "reference.json"))
	if string(prior) != string(after) {
		t.Fatal("last good reference changed")
	}
	if err := os.WriteFile(filepath.Join(in.Out, "openapi.json.gz"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reference.OpenCatalog(context.Background(), in.Out); err == nil {
		t.Fatal("corrupt reference accepted")
	}
}

func TestBuildRejectsUnqualifiedOrMismatchedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"old receipt", func(r map[string]any) { r["version"] = 1 }},
		{"wrong workflow", func(r map[string]any) { r["kind"] = "project-tag-workflows" }},
		{"failed", func(r map[string]any) { r["passed"] = false }},
		{"cleanup", func(r map[string]any) { r["cleanup"] = false }},
		{"binary", func(r map[string]any) { r["testBinarySha256"] = strings.Repeat("b", 64) }},
		{"image", func(r map[string]any) { r["imageId"] = "sha256:" + strings.Repeat("b", 64) }},
		{"modules", func(r map[string]any) { r["moduleInventory"].(map[string]any)["sha256"] = strings.Repeat("b", 64) }},
		{"parser", func(r map[string]any) { r["catalog"].(map[string]any)["parserVersion"] = "old" }},
		{"old identity policy", func(r map[string]any) { r["catalog"].(map[string]any)["contractPolicy"] = "igw-contract/1" }},
		{"contract", func(r map[string]any) { r["catalog"].(map[string]any)["contractSha256"] = strings.Repeat("b", 64) }},
		{"source", func(r map[string]any) { r["catalog"].(map[string]any)["sourceKind"] = "import" }},
		{"checks", func(r map[string]any) { r["checks"] = []any{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := fixtureInputs(t)
			b, _ := os.ReadFile(in.Resources)
			var r map[string]any
			_ = json.Unmarshal(b, &r)
			tc.edit(r)
			b, _ = json.Marshal(r)
			if err := os.WriteFile(in.Resources, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Build(context.Background(), in); err == nil {
				t.Fatal("unqualified evidence accepted")
			}
			if _, err := os.Stat(in.Out); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed qualification published output")
			}
		})
	}
}

func TestReferenceRejectsManifestTraversalAndIncompletePublication(t *testing.T) {
	in := fixtureInputs(t)
	m, err := Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(in.Out, "reference.json")
	m.Files[0].Path = "../capture.json"
	b, _ := json.Marshal(m)
	_ = os.WriteFile(path, b, 0600)
	if _, err := reference.Read(context.Background(), in.Out); err == nil {
		t.Fatal("reference payload escaped its directory")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := reference.Read(context.Background(), in.Out); err == nil {
		t.Fatal("incomplete publication accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, fixtureInputs(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestQualificationRequiresLifecycleAndPartialFailureEvidence(t *testing.T) {
	for _, mode := range []string{"lifecycle", "partial"} {
		t.Run(mode, func(t *testing.T) {
			in := fixtureInputs(t)
			path := in.Lifecycle
			if mode == "partial" {
				path = in.Transfers
			}
			b, _ := os.ReadFile(path)
			var r map[string]any
			_ = json.Unmarshal(b, &r)
			if mode == "lifecycle" {
				r["checks"] = []string{"owned-cleanup"}
			} else {
				for _, check := range r["checks"].([]any) {
					c := check.(map[string]any)
					if c["name"] == "workflow-tag-abort" {
						c["outcome"] = "completed"
					}
				}
			}
			b, _ = json.Marshal(r)
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Build(context.Background(), in); err == nil {
				t.Fatal("qualification accepted missing containment or partial-failure evidence")
			}
		})
	}
}

func TestReferenceVerifiesDecompressedVendorIdentity(t *testing.T) {
	for _, malformed := range []bool{true, false} {
		in := fixtureInputs(t)
		m, err := Build(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte("invalid gzip")
		if !malformed {
			var b bytes.Buffer
			w := gzip.NewWriter(&b)
			_, _ = w.Write([]byte(strings.Replace(syntheticDocument, "OK", "Different", 1)))
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			payload = b.Bytes()
		}
		for n, file := range m.Files {
			if file.Path == "openapi.json.gz" {
				m.Files[n].Bytes = int64(len(payload))
				m.Files[n].SHA256 = digest(payload)
			}
		}
		b, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(in.Out, "reference.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(in.Out, "openapi.json.gz"), payload, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := reference.OpenCatalog(context.Background(), in.Out); err == nil {
			t.Fatal("compressed payload checksum bypassed vendor identity validation")
		}
	}
}
