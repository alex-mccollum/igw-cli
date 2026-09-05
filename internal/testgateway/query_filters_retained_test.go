package testgateway_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

// Historical receipt integrity and current parsing are separate checks. This
// test never renews the recorded parser, executable, or observation timestamps.
func TestRetainedQueryFilterEvidence(t *testing.T) {
	const source = "c8974b78eb1a0483fd2208612b769f644d82d7d7"
	const binary = "30d85edb188e8e228afe2d98c562daca3103954e2cbd82952b77cfa7019ac7de"
	read := func(t *testing.T, path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "query-filters", path))
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
			var receipt struct {
				Version                                                          int
				Kind, Image, ImageID, Platform, GatewayVersion, TestBinarySHA256 string
				ModuleInventory                                                  *moduleprofile.Inventory
				ModuleWhitelist                                                  []string
				StartedAt, FinishedAt                                            time.Time
				Catalog                                                          catalog.Metadata
				OpenAPI                                                          artifact.Info
				Checks                                                           []transferCheck
				Cleanup, Passed                                                  bool
			}
			decode(t, tt.version+"/query/query-filters.json", &receipt)
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
				LifecycleExitCode, QueryExitCode int
				IndependentCleanupVerified       bool
			}
			decode(t, tt.version+"/process.json", &process)
			if receipt.Version != 1 || receipt.Kind != "query-filter-workflows" || !receipt.Passed || !receipt.Cleanup || !lifecycle.Passed || !lifecycle.Cleanup || process.LifecycleExitCode != 0 || process.QueryExitCode != 0 || !process.IndependentCleanupVerified {
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
			if !reflect.DeepEqual(receipt.Checks, retainedQueryChecks()) {
				t.Fatal("live check outcomes or request counts changed")
			}
			for _, stage := range []string{"after-lifecycle.txt", "after-query.txt"} {
				if strings.TrimSpace(string(read(t, tt.version+"/"+stage))) != "" {
					t.Fatal("independent query found a qualification container")
				}
			}
			compressed := read(t, tt.version+"/query/openapi.json.gz")
			if int64(len(compressed)) != receipt.OpenAPI.Bytes || fmt.Sprintf("%x", sha256.Sum256(compressed)) != receipt.OpenAPI.SHA256 {
				t.Fatal("compressed vendor artifact changed")
			}
			gz, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(io.LimitReader(gz, catalog.MaxDocumentBytes+1))
			closeErr := gz.Close()
			if err != nil || closeErr != nil {
				t.Fatal("cannot read captured vendor document")
			}
			c, err := catalog.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			m := receipt.Catalog
			if m.Version != 2 || m.SourceKind != "gateway" || m.ParserVersion != "libopenapi/0.38.7+validator/0.14.0;igw/13" || m.ContractPolicy != "igw-contract/2" || m.ContractSHA256 != tt.contract || m.RawSHA256 != c.RawHash() || m.DocumentSHA256 != c.DocumentHash() || m.ContractSHA256 != c.ContractHash() || m.FetchedAt.Before(receipt.StartedAt) || m.VerifiedAt.Before(m.FetchedAt) || m.VerifiedAt.After(receipt.FinishedAt) {
				t.Fatal("catalog provenance does not match the original vendor bytes")
			}
			for _, path := range []string{"/data/api/v1/resources/list/ignition/schedule", "/data/api/v1/projects/list", "/data/api/v1/logs"} {
				query := url.Values{"limit": {"1"}, "offset": {"0"}, "name[eq]": {"filter + & % # / = 日本"}}
				req, _ := http.NewRequest("GET", "http://gateway.test"+path+"?"+query.Encode(), nil)
				issues, err := c.Validate("GET "+path, req)
				if err != nil || len(issues) != 0 {
					t.Fatalf("current parser cannot validate captured filter shape: %+v %v", issues, err)
				}
			}
		})
	}
}

func retainedQueryChecks() []transferCheck {
	checks := []transferCheck{{Name: "catalog", Outcome: "completed"}}
	for _, name := range []string{"alpha", "beta"} {
		checks = append(checks, transferCheck{Name: "create-resource-igw-filter-" + name, Outcome: "completed", HTTPStatus: 200, OperationRequests: 3})
	}
	for _, name := range []string{"resource-name-eq", "resource-exact-description", "resource-multiple-filters", "resource-filter-pagination", "resource-nonmatch", "generic-resource-filter"} {
		checks = append(checks, transferCheck{Name: name, Outcome: "completed", HTTPStatus: 200, OperationRequests: 1})
	}
	checks = append(checks, transferCheck{Name: "generic-filter-preview", Outcome: "preview"})
	for _, name := range []string{"alpha", "beta"} {
		checks = append(checks, transferCheck{Name: "create-project-igw-filter-" + name, Outcome: "accepted", HTTPStatus: 200, OperationRequests: 1})
	}
	for _, name := range []string{"project-name-eq", "project-multiple-filters", "project-nonmatch", "logs-baseline", "logs-logger-eq", "logs-nonmatch"} {
		checks = append(checks, transferCheck{Name: name, Outcome: "completed", HTTPStatus: 200, OperationRequests: 1})
	}
	for _, command := range []string{"resource", "project", "logs"} {
		checks = append(checks, transferCheck{Name: command + "-invalid-operator", Outcome: "failed", ErrorKind: "validation", ExitCode: 2}, transferCheck{Name: command + "-duplicate-key", Outcome: "failed", ErrorKind: "usage", ExitCode: 2})
	}
	return checks
}
