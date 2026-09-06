package testgateway_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

// Historical receipt integrity and current parsing are separate checks. This
// test never renews the recorded parser, executable, or observation timestamps.
func TestRetainedBodyInputEvidence(t *testing.T) {
	const source = "03566a80e1fa5458fe36b969e032c7cd171d1ad8"
	const binary = "79305d68639e7717ec5ecb90f3b794273ad23915311df33df137dcc515cfb003"
	read := func(t *testing.T, path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "body-inputs", path))
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
	checkInputEvidenceManifest(t, read(t, "manifest.json"), "b783651e7334b4381a491785a27403d6695e14fbd7ec00d6daa16043c863c704", read)
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
	if before.Commit != source || after.Commit != source || before.Dirty || after.Dirty || before.ObservedAt.IsZero() || !after.ObservedAt.After(before.ObservedAt) || build.SourceCommit != source || build.SourceDirty || !build.Trimpath || build.TestBinarySHA256 != binary || build.GoVersion != "go1.27.1" || build.FinishedAt.Before(before.ObservedAt) {
		t.Fatal("clean source or executable provenance changed")
	}
	if strings.TrimSpace(string(read(t, "preflight-containers.txt"))) != "" {
		t.Fatal("qualification started with a leftover container")
	}
	for _, tt := range []struct{ version, image, imageID, contract string }{
		{"8.3.0", "9fa22bb89a3004b95c6d7b281f690f9122ec53e4509f762fb23e10d5306eeafb", "3ccb0dd03f8237048a0cc8554abd29a25429d684be85bbbe0c8bd1c42a95be85", "d5ec3e55a0d85d8e22bf8a7389307fa36b78362c5b71ee01947146c57c2ebb75"},
		{"8.3.9", "28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2", "f28a0c5a7a85dab32f0f0a04a80bc4dfd00a27de12f899bef979ffb9cce967fd", "2112cbe1a55bc7b92add06cd89e86bda987ce7d7855e5bdee06660dc9f9a45fe"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			var receipt inputReceipt
			decode(t, tt.version+"/inputs/body-inputs.json", &receipt)
			var lifecycle struct {
				Version, LifetimeSeconds, ExitCode         int
				Kind                                       string
				ElapsedSeconds                             float64
				Image, ImageID, Platform, TestBinarySHA256 string
				ModuleWhitelist                            []string
				StartedAt, FinishedAt                      time.Time
				Checks                                     []string
				Cleanup, Passed                            bool
			}
			decode(t, tt.version+"/lifecycle.json", &lifecycle)
			var process struct {
				LifecycleExitCode, InputExitCode int
				IndependentCleanupVerified       bool
			}
			decode(t, tt.version+"/process.json", &process)
			if receipt.Version != 1 || receipt.Kind != "request-body-inputs" || !receipt.Passed || !receipt.Cleanup || !lifecycle.Passed || !lifecycle.Cleanup || process.LifecycleExitCode != 0 || process.InputExitCode != 0 || !process.IndependentCleanupVerified {
				t.Fatal("incomplete live qualification or cleanup evidence")
			}
			wantLifecycle := []string{"image-platform", "container-image-identity", "configured-limits", "kernel-limits", "loopback-only", "exclusive-admission", "lifetime-termination", "no-oom", "owned-cleanup", "absence-after-cleanup"}
			if lifecycle.Version != 1 || lifecycle.Kind != "capture-lifecycle" || lifecycle.LifetimeSeconds != 5 || lifecycle.ElapsedSeconds < 4 || lifecycle.ElapsedSeconds > 25 || (lifecycle.ExitCode != 124 && lifecycle.ExitCode != 137) || !reflect.DeepEqual(lifecycle.Checks, wantLifecycle) {
				t.Fatal("lifecycle containment evidence changed")
			}
			if receipt.Image != "inductiveautomation/ignition@sha256:"+tt.image || receipt.ImageID != "sha256:"+tt.imageID || receipt.Platform != "linux/amd64" || !strings.HasPrefix(receipt.GatewayVersion, tt.version+" ") || receipt.TestBinarySHA256 != binary || lifecycle.Image != receipt.Image || lifecycle.ImageID != receipt.ImageID || lifecycle.Platform != receipt.Platform || lifecycle.TestBinarySHA256 != binary {
				t.Fatal("image or executable identities differ")
			}
			if lifecycle.StartedAt.Before(build.FinishedAt) || !lifecycle.FinishedAt.After(lifecycle.StartedAt) || receipt.StartedAt.Before(lifecycle.FinishedAt) || !receipt.FinishedAt.After(receipt.StartedAt) || after.ObservedAt.Before(receipt.FinishedAt) {
				t.Fatal("qualification timestamps are inconsistent")
			}
			profile, _ := moduleprofile.Select("core-opcua")
			if err := profile.ValidateInventory(receipt.ModuleInventory); err != nil {
				t.Fatal(err)
			}
			if len(receipt.ModuleInventory.Modules) != 32 || !reflect.DeepEqual(receipt.ModuleWhitelist, profile.EnabledModules) || !reflect.DeepEqual(lifecycle.ModuleWhitelist, receipt.ModuleWhitelist) || receipt.ModuleInventory.ObservedAt.Before(receipt.StartedAt) || receipt.ModuleInventory.ObservedAt.After(receipt.FinishedAt) {
				t.Fatal("observed module profile changed")
			}
			checkRetainedBodyInputs(t, receipt.Checks, tt.version)
			for _, stage := range []string{"after-lifecycle.txt", "after-inputs.txt"} {
				if strings.TrimSpace(string(read(t, tt.version+"/"+stage))) != "" {
					t.Fatal("independent query found a qualification container")
				}
			}
			compressed := read(t, tt.version+"/inputs/openapi.json.gz")
			if receipt.OpenAPI == nil || receipt.Catalog == nil {
				t.Fatal("missing captured catalog")
			}
			if int64(len(compressed)) != receipt.OpenAPI.Bytes || fmt.Sprintf("%x", sha256.Sum256(compressed)) != receipt.OpenAPI.SHA256 {
				t.Fatal("compressed vendor artifact changed")
			}
			gz, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(io.LimitReader(gz, catalog.MaxDocumentBytes+1))
			closeErr := gz.Close()
			if err != nil || closeErr != nil || len(raw) > catalog.MaxDocumentBytes {
				t.Fatal("cannot read captured vendor document")
			}
			c, err := catalog.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			m := receipt.Catalog
			if m.Version != 2 || m.SourceKind != "gateway" || m.ParserVersion != "libopenapi/0.38.7+validator/0.14.0;igw/19" || m.ContractPolicy != "igw-contract/2" || m.ContractSHA256 != tt.contract || m.RawSHA256 != c.RawHash() || m.DocumentSHA256 != c.DocumentHash() || m.ContractSHA256 != c.ContractHash() || m.FetchedAt.Before(receipt.StartedAt) || m.VerifiedAt.Before(m.FetchedAt) || m.VerifiedAt.After(receipt.FinishedAt) {
				t.Fatal("catalog provenance does not match the original vendor bytes")
			}
			for _, log := range []string{"lifecycle.log", "inputs.log"} {
				if !strings.HasSuffix(string(read(t, tt.version+"/"+log)), "PASS\n") {
					t.Fatal("live process log did not end successfully")
				}
			}
		})
	}
}

// The anchored manifest covers every original file, including fields that are
// intentionally not interpreted by these focused semantic checks.
func checkInputEvidenceManifest(t *testing.T, raw []byte, expectedSHA256 string, read func(*testing.T, string) []byte) {
	t.Helper()
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != expectedSHA256 {
		t.Fatal("original evidence manifest changed")
	}
	var manifest struct {
		Version int
		Files   []struct {
			Path   string
			Bytes  int64
			SHA256 string
		}
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || len(manifest.Files) != 21 {
		t.Fatal("incomplete original evidence manifest")
	}
	seen := make(map[string]bool)
	for _, file := range manifest.Files {
		if !filepath.IsLocal(file.Path) || seen[file.Path] {
			t.Fatal("invalid original evidence path")
		}
		seen[file.Path] = true
		b := read(t, file.Path)
		if int64(len(b)) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(b)) != file.SHA256 {
			t.Fatalf("original evidence changed: %s", file.Path)
		}
	}
}

func checkRetainedBodyInputs(t *testing.T, checks []inputCheck, version string) {
	t.Helper()
	wantCount := 31
	if version == "8.3.9" {
		wantCount = 62
	}
	if len(checks) != wantCount {
		t.Fatalf("got %d checks, want %d", len(checks), wantCount)
	}
	byName := make(map[string]inputCheck)
	for _, check := range checks {
		if _, exists := byName[check.Name]; exists || check.Name == "" || check.OperationRequests != int64(len(check.Wire)) {
			t.Fatal("duplicate check or inconsistent observed request count")
		}
		byName[check.Name] = check
		if check.Preview != nil && (check.Outcome != "preview" || check.OperationRequests != 0 || !check.Preview.BodyPresent) {
			t.Fatalf("preview dispatched an operation or lost input presence: %s", check.Name)
		}
	}
	get := func(name string) inputCheck {
		t.Helper()
		check, ok := byName[name]
		if !ok {
			t.Fatalf("missing check %s", name)
		}
		return check
	}
	outcome := func(name, result, kind string, status, code, requests int) inputCheck {
		t.Helper()
		check := get(name)
		if check.Outcome != result || check.ErrorKind != kind || check.HTTPStatus != status || check.ExitCode != code || check.OperationRequests != int64(requests) {
			t.Fatalf("unexpected outcome: %+v", check.transferCheck)
		}
		return check
	}
	outcome("catalog", "completed", "", 0, 0, 0)
	const textHash = "c34bfd5751034b38e4a33b686a8f95e02ddeb0cd752356b8c84a2b6b85e3cff0"
	const binaryHash = "cc768f061995b37aec5995eaafa6d8bdb0d3efe6a659e547793b1c41d41a662e"
	const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	for _, name := range []string{"text-literal", "text-file", "text-stdin", "text-empty-literal", "text-empty-file", "text-empty-stdin", "binary-body", "binary-upload", "binary-empty-upload"} {
		media, coverage, hash, size := "text/plain", "declared_schema", textHash, int64(40)
		if name == "text-file" {
			media += "; charset=utf-8"
		}
		if strings.HasPrefix(name, "binary-") {
			media, coverage, hash, size = "application/octet-stream", "declared_transport", binaryHash, 65536
		}
		if strings.Contains(name, "empty") {
			hash, size = emptyHash, 0
		}
		preview := outcome(name+"-preview", "preview", "", 0, 0, 0)
		execution := outcome(name, "accepted", "", 200, 0, 1)
		p := preview.Preview
		w := execution.Wire[0]
		if p == nil || p.Method != "POST" || p.Path != "/data/api/v1/encryption/encrypt" || !p.Mutating || p.BodyBytes != size || p.BodySHA256 != hash || p.ContentType != media || p.Validation != coverage || preview.Validation != coverage || execution.Validation != coverage || w.Method != p.Method || w.Path != p.Path || w.BodyBytes != size || w.ContentLength != size || w.BodySHA256 != hash || w.ContentType != media {
			t.Fatalf("prepared and consumed input identity differ: %s", name)
		}
	}
	for _, negative := range []struct{ name, kind string }{
		{"missing-body", "validation"}, {"undeclared-json", "validation"}, {"invalid-utf8", "validation"}, {"unsupported-charset", "unsupported_input"}, {"unsupported-text-stream", "usage"},
	} {
		for _, mode := range []string{"--dry-run", "--yes"} {
			outcome(negative.name+mode, "failed", negative.kind, 0, 2, 0)
		}
	}
	if version == "8.3.0" {
		for _, mode := range []string{"--dry-run", "--yes"} {
			outcome("bulk-unavailable"+mode, "failed", "usage", 0, 2, 0)
		}
		return
	}
	preview := outcome("multipart-preview", "preview", "", 0, 0, 0)
	p := preview.Preview
	if p == nil || len(p.Parts) != 3 || p.BodyBytes != 66201 || p.Method != "PUT" || p.Path != "/data/api/v1/resources/datafile/ignition/translations" || p.Validation != "declared_transport" || !reflect.DeepEqual(p.QueryKeys, []string{"collection", "signature"}) {
		t.Fatal("multipart preview identity changed")
	}
	for _, name := range []string{"multipart-create", "multipart-stale-signature"} {
		result, kind, status, code := "accepted", "", 200, 0
		if name == "multipart-stale-signature" {
			result, kind, status, code = "failed", "http", 500, 7
		}
		check := outcome(name, result, kind, status, code, 1)
		w := check.Wire[0]
		if check.Validation != "declared_transport" || w.Method != p.Method || w.Path != p.Path || w.BodyBytes != p.BodyBytes || w.ContentLength != p.BodyBytes || !strings.HasPrefix(w.ContentType, "multipart/form-data; boundary=") {
			t.Fatalf("multipart transmission changed: %s", name)
		}
	}
	for i, fixture := range []struct {
		name, hash, media string
		size              int64
	}{
		{"igw-input-binary.bin", binaryHash, "application/octet-stream", 65536},
		{"igw-input-text.txt", textHash, "text/plain; charset=utf-8", 40},
		{"igw-input-empty.bin", emptyHash, "application/octet-stream", 0},
	} {
		part := p.Parts[i]
		if part.Name != "files" || part.Filename != fixture.name || part.SHA256 != fixture.hash || part.Bytes != fixture.size || part.ContentType != fixture.media {
			t.Fatal("multipart preview lost a file identity")
		}
		for _, stage := range []string{"before", "after-preview", "after-delete"} {
			outcome("files-"+stage+"-"+fixture.name, "failed", "http", 404, 7, 1)
		}
		for _, stage := range []string{"created", "after-stale", "overwritten"} {
			check := outcome("files-"+stage+"-"+fixture.name, "completed", "", 200, 0, 1)
			hash, size := fixture.hash, fixture.size
			if stage == "overwritten" && i == 0 {
				hash, size = textHash, 40
			}
			if check.Artifact == nil || check.Artifact.Bytes != size || check.Artifact.SHA256 != hash {
				t.Fatalf("downloaded bytes differ: %s", check.Name)
			}
		}
		outcome("signature-delete-"+fixture.name, "completed", "", 200, 0, 1)
		outcome("delete-"+fixture.name, "accepted", "", 200, 0, 1)
	}
	overwrite := outcome("single-file-overwrite", "accepted", "", 200, 0, 1)
	if overwrite.Wire[0].BodySHA256 != textHash || overwrite.Wire[0].BodyBytes != 40 || overwrite.Validation != "declared_transport" {
		t.Fatal("opaque overwrite changed")
	}
	for _, name := range []string{"translations-before", "translations-signature", "translations-after-create", "translations-after-stale", "translations-after-files"} {
		outcome(name, "completed", "", 200, 0, 1)
	}
}
