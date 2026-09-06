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

func TestRetainedRestartEvidence(t *testing.T) {
	const source = "8084cd084fdc08cb925b4c31f628d94ceda29b05"
	const binary = "b3df1820551216061df75de662f4a01a10fdf0d155350174983c521548f07457"
	read := func(t *testing.T, path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "restart", path))
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
	checkInputEvidenceManifest(t, read(t, "manifest.json"), "23158fb00bc83fab80029839d9009e5bc4f514edc3829f9ddfa0104be69fb5c8", read)
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
			decode(t, version+"/restart/restart.json", &receipt)
			var lifecycle struct {
				Kind, Image, ImageID, Platform, TestBinarySHA256 string
				Checks                                           []string
				StartedAt, FinishedAt                            time.Time
				LifetimeSeconds, ExitCode                        int
				Passed, Cleanup                                  bool
			}
			decode(t, version+"/lifecycle.json", &lifecycle)
			var process struct {
				LifecycleExitCode, RestartExitCode int
				IndependentCleanupVerified         bool
			}
			decode(t, version+"/process.json", &process)
			if receipt.Version != 1 || receipt.Kind != "gateway-restart" || !receipt.Passed || !receipt.Cleanup || !lifecycle.Passed || !lifecycle.Cleanup || process.LifecycleExitCode != 0 || process.RestartExitCode != 0 || !process.IndependentCleanupVerified {
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
			for _, path := range []string{"after-lifecycle.txt", "after-restart.txt"} {
				if len(read(t, version+"/"+path)) != 0 {
					t.Fatal("independent cleanup query found a container")
				}
			}
			for _, path := range []string{"lifecycle.log", "restart.log"} {
				if !strings.HasSuffix(string(read(t, version+"/"+path)), "PASS\n") {
					t.Fatal("live process did not pass")
				}
			}
			checkRetainedRestartChecks(t, receipt, version)
			compressed := read(t, version+"/restart/openapi.json.gz")
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

func checkRetainedRestartChecks(t *testing.T, receipt inputReceipt, version string) {
	t.Helper()
	checks := receipt.Checks
	restartRequests, polls := 75, 61
	if version == "8.3.0" {
		restartRequests, polls = 76, 62
	}
	want := []struct {
		name, outcome            string
		code, requests, catalogs int
	}{
		{"catalog", "completed", 0, 0, 1},
		{"confirmation", "failed", 2, 0, 0},
		{"conflicting-flags", "failed", 2, 0, 0},
		{"offline-preview", "failed", 2, 0, 0},
		{"preview", "preview", 0, 4, 0},
		{"restart", "completed", 0, restartRequests, 1},
		{"tasks-after", "completed", 0, 1, 0},
		{"doctor-after", "completed", 0, 1, 0},
		{"preview-after", "preview", 0, 4, 0},
	}
	if len(checks) != len(want) {
		t.Fatal("restart checks missing")
	}
	for index, expected := range want {
		got := checks[index]
		if got.Name != expected.name || got.Outcome != expected.outcome || got.ExitCode != expected.code || got.OperationRequests != int64(expected.requests) || len(got.Wire) != expected.requests || got.CatalogRequests != expected.catalogs {
			t.Fatalf("restart check changed: %s", expected.name)
		}
		writes := 0
		for _, wire := range got.Wire {
			if wire.BodyBytes != 0 || wire.BodySHA256 != inputDigest(nil) || wire.ContentLength != 0 {
				t.Fatal("restart workflow changed request bodies")
			}
			if wire.Method == "POST" {
				writes++
				if expected.name != "restart" || wire.Path != "/data/api/v1/restart-tasks/restart" {
					t.Fatal("unexpected restart mutation")
				}
			} else if wire.Method != "GET" {
				t.Fatal("unexpected restart method")
			}
		}
		if (expected.name == "restart" && writes != 1) || (expected.name != "restart" && writes != 0) {
			t.Fatal("restart replay or preview mutation")
		}
	}
	preview, restart, last := checks[4].Restart, checks[5].Restart, checks[8].Restart
	if preview == nil || restart == nil || last == nil || preview.Request == nil || preview.Request.Method != "POST" || !preview.Request.Mutating || preview.ConfirmedRequests != 0 || last.ConfirmedRequests != 0 || restart.ConfirmedRequests != 1 || !restart.Acknowledged || restart.Proof != "uptime_reset" || restart.Polls != polls || restart.Correlation != "observed_node" {
		t.Fatal("original restart evidence changed")
	}
	if restart.Before.ProcessID != restart.Last.ProcessID || restart.Last.Uptime >= restart.Before.Uptime || restart.Before.PendingCount != 0 || restart.Last.PendingCount != 0 || restart.Before.NodeSHA256 != restart.Last.NodeSHA256 || preview.Before.ProcessID != restart.Before.ProcessID || last.Before.ProcessID != restart.Last.ProcessID {
		t.Fatal("API did not observe uptime reset with consistent reported identity")
	}
	// Both independently commissioned containers returned this same identifier.
	// It must never be described as proof of globally unique node identity.
	if restart.Before.NodeSHA256 != "fb8a6402175bd59d18a885c9e379749a5097fb0fca9b8b09e60272b346143ae6" {
		t.Fatal("reported localId observation changed")
	}
	before, after := checks[4].Process, checks[5].Process
	if before == nil || after == nil || !before.ContainmentVerified || !after.ContainmentVerified || len(before.ContainerID) != 64 || before.ContainerID != after.ContainerID || !before.ContainerStartedAt.Equal(after.ContainerStartedAt) || before.ContainerRestarts != 0 || after.ContainerRestarts != 0 || before.ReportedProcessID != restart.Before.ProcessID || after.ReportedProcessID != restart.Last.ProcessID || before.ReportedExecutable != "ignition-gateway" || after.ReportedExecutable != "ignition-gateway" || before.ExecutableName != "java" || after.ExecutableName != "java" || before.ProcessID == after.ProcessID || before.StartTicks == 0 || after.StartTicks <= before.StartTicks || before.ReportedStartTicks != after.ReportedStartTicks || before.ReportedStartTicks == 0 {
		t.Fatal("independent JVM restart or unchanged wrapper/container evidence missing")
	}
	if before.ObservedAt.Before(receipt.StartedAt) || !after.ObservedAt.After(before.ObservedAt) || receipt.FinishedAt.Before(after.ObservedAt) {
		t.Fatal("process observations escaped the live run")
	}
}
