package testgateway_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/resource"
)

type journeyExecutable struct {
	SHA256 string         `json:"sha256"`
	Checks []journeyCheck `json:"checks"`
}

// Native process evidence deliberately has no operation-request count: the
// subprocess owns its transport. Existing observed suites cover wire counts.
type journeyCheck struct {
	Name         string         `json:"name"`
	Mode         string         `json:"mode"`
	ExitCode     int            `json:"exitCode"`
	Outcome      string         `json:"outcome,omitempty"`
	ErrorKind    string         `json:"errorKind,omitempty"`
	Verification string         `json:"verification,omitempty"`
	OutputSHA256 string         `json:"outputSha256"`
	OutputBytes  int            `json:"outputBytes"`
	Artifact     *inputArtifact `json:"artifact,omitempty"`
}

func TestLiveWorkflowJourneys(t *testing.T) {
	if os.Getenv("IGW_ACCEPTANCE_TEST_IMAGE") == "" {
		t.Skip("requires a pinned image and guarded live invocation")
	}
	binary := os.Getenv("IGW_JOURNEY_CLI_BINARY")
	if !filepath.IsAbs(binary) {
		t.Fatal("requires an absolute path to the prebuilt IGW_JOURNEY_CLI_BINARY")
	}
	f, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, hashErr := io.Copy(hash, f)
	closeErr := f.Close()
	if hashErr != nil || closeErr != nil {
		t.Fatal("cannot hash CLI executable")
	}
	s := beginObservedSuite(t, "IGW_JOURNEY_EVIDENCE_DIR", "workflow-journeys", "journeys.json")
	s.receipt.Executable = &journeyExecutable{SHA256: hex.EncodeToString(hash.Sum(nil)), Checks: []journeyCheck{}}
	root := t.TempDir()
	env := journeyEnvironment(os.Environ(), root)
	_, secret, _ := strings.Cut(s.token, ":")
	invoke := func(name, input string, human bool, args ...string) (transferResult, string, int) {
		t.Helper()
		mode := "json"
		if human {
			mode = "human"
		} else {
			args = append(args, "--json")
		}
		cmd := exec.CommandContext(s.ctx, binary, append(args, "--timeout", "90s")...)
		cmd.Env = env
		cmd.Stdin = strings.NewReader(input)
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatal("CLI process could not complete")
			}
			code = exited.ExitCode()
		}
		if strings.Contains(out.String()+stderr.String(), s.token) || secret != "" && strings.Contains(out.String()+stderr.String(), secret) {
			t.Fatal("CLI process exposed its credential")
		}
		check := journeyCheck{Name: name, Mode: mode, ExitCode: code, OutputSHA256: inputDigest(out.Bytes()), OutputBytes: out.Len()}
		var got transferResult
		if !human {
			var version struct{ Version string }
			if json.Unmarshal(out.Bytes(), &got) != nil || json.Unmarshal(out.Bytes(), &version) != nil || version.Version != "igw/v1" || got.OK != (code == 0) || stderr.Len() != 0 {
				t.Fatalf("%s: invalid executable JSON contract", name)
			}
			check.Outcome, check.Verification = got.Outcome, got.Meta.Verification
			if got.Error != nil {
				check.ErrorKind = got.Error.Kind
				if got.Error.Code != code {
					t.Fatal("process exit disagrees with JSON error")
				}
			}
			if got.Artifact != nil {
				check.Artifact = &inputArtifact{Bytes: got.Artifact.Bytes, SHA256: got.Artifact.SHA256}
			}
			if got.Meta.Catalog != nil && got.Meta.Catalog.ContractSHA256 != s.receipt.Catalog.ContractSHA256 {
				t.Fatal("executable used a different Gateway contract")
			}
		}
		s.receipt.Executable.Checks = append(s.receipt.Executable.Checks, check)
		t.Logf("%s: %s exit=%d outcome=%s", name, mode, code, got.Outcome)
		return got, out.String(), code
	}
	run := func(name string, args ...string) transferResult {
		got, _, _ := invoke(name, "", false, args...)
		return got
	}
	ok := func(name string, args ...string) transferResult {
		t.Helper()
		got := run(name, args...)
		if !got.OK {
			t.Fatalf("%s failed: %s", name, got.Error.Kind)
		}
		return got
	}
	human := func(name, want string, args ...string) {
		t.Helper()
		_, text, code := invoke(name, "", true, args...)
		if code != 0 || !strings.Contains(text, want) {
			t.Fatalf("%s: human guidance missing or failed", name)
		}
	}
	verified := func(name string, args ...string) resource.Evidence {
		t.Helper()
		got := ok(name, args...)
		var evidence resource.Evidence
		if got.Outcome != "completed" || got.Meta.Verification != "verified" || json.Unmarshal(got.Data, &evidence) != nil {
			t.Fatalf("%s: state not verified", name)
		}
		return evidence
	}

	human("offline-help", "Available Commands:", "--help")
	ok("offline-schema", "schema", "resource", "update")
	ok("profile-preview", "profile", "set", "qualification", "--url", s.session.URL, "--use", "--dry-run")
	setup, _, _ := invoke("profile-setup", s.token, false, "profile", "set", "qualification", "--url", s.session.URL, "--use", "--token-stdin", "--yes")
	if !setup.OK {
		t.Fatal("profile setup failed")
	}
	ok("profile-show", "profile", "show")
	ok("doctor-json", "gateway", "doctor")
	human("doctor-human", "Gateway information request succeeded.", "gateway", "doctor")
	ok("operation-discovery", "api", "list", "--search", "gateway")
	ok("operation-contract", "api", "describe", "GET /data/api/v1/gateway-info")
	ok("offline-reference", "api", "list", "--reference", "ignition-8.3.9-core", "--search", "gateway")

	const body = `{"description":"workflow journey","enabled":true,"config":{"profile":{"type":"basic schedule"},"settings":{"allDays":true,"allDayTime":"01:00-02:00"}}}`
	const kind, name = "ignition/schedule", "igw-journey"
	ok("resource-description", "resource", "describe", kind)
	human("resource-preview", "Preview: no proposed mutation was sent.", "resource", "create", kind, name, "--body", body, "--dry-run")
	absent := run("resource-preview-absence", "resource", "get", kind, name)
	if absent.OK || inputHTTPStatus(absent) != 404 {
		t.Fatal("preview changed resource")
	}
	created := verified("resource-create", "resource", "create", kind, name, "--body", body, "--yes")
	human("resource-update-preview", "--if-signature", "resource", "update", kind, name, "--body", `{"description":"updated journey"}`, "--dry-run")
	updated := verified("resource-update", "resource", "update", kind, name, "--body", `{"description":"updated journey"}`, "--if-signature", created.AfterSignature, "--yes")
	stale := run("resource-stale-review", "resource", "update", kind, name, "--body", body, "--if-signature", created.AfterSignature, "--yes")
	if stale.OK || stale.Error == nil || stale.Error.Kind != "conflict" {
		t.Fatal("stale review was accepted")
	}
	readback := ok("resource-readback", "resource", "get", kind, name)
	var state struct{ Description, Signature string }
	if json.Unmarshal(readback.Data, &state) != nil || state.Description != "updated journey" || state.Signature != updated.AfterSignature {
		t.Fatal("resource readback changed")
	}
	verified("resource-delete", "resource", "delete", kind, name, "--if-signature", updated.AfterSignature, "--yes")

	capabilities := ok("capabilities", "api", "capabilities")
	var assessed []catalog.CapabilityAssessment
	if json.Unmarshal(capabilities.Data, &assessed) != nil {
		t.Fatal("invalid capabilities")
	}
	tagsAvailable, err := reference.TagRoundTripAvailable(assessed)
	if err != nil {
		t.Fatal(err)
	}
	const sourceName = "igw-journey-source"
	ok("project-create", "api", "request", "POST /data/api/v1/projects", "--body", `{"name":"igw-journey-source","description":"transfer qualification","title":"Qualification","enabled":false}`, "--yes")
	source := filepath.Join(root, "source.zip")
	ok("project-source-export", "project", "export", sourceName, "--out", source)
	qualifyTransferWorkflows(t, run, root, sourceName, source, tagsAvailable)
	if !tagsAvailable {
		qualifyUnavailableTags(t, run, root)
	}

	// The fresh Gateway's logs establish the timestamp unit and useful query
	// semantics independently of the human formatter's fixture expectations.
	until := time.Now().UTC()
	since := until.Add(-time.Hour)
	window := []string{"--since", since.Format(time.RFC3339Nano), "--until", until.Format(time.RFC3339Nano)}
	logs := ok("logs-json", append([]string{"logs", "list", "--limit", "5", "--min-level", "INFO"}, window...)...)
	var page struct {
		Items []struct {
			Timestamp                  int64
			Level, LoggerName, Message string
		}
	}
	if json.Unmarshal(logs.Data, &page) != nil || len(page.Items) == 0 {
		t.Fatal("no events for troubleshooting qualification")
	}
	row := page.Items[0]
	instant := time.UnixMilli(row.Timestamp)
	if instant.Before(since) || instant.After(until) || row.LoggerName == "" {
		t.Fatal("log time unit/window or logger did not match")
	}
	filtered := append([]string{"logs", "list", "--limit", "5", "--logger", row.LoggerName, "--min-level", "INFO"}, window...)
	human("logs-human", instant.UTC().Format(time.RFC3339Nano), filtered...)
	searched := ok("logs-search", append(filtered, "--search", row.Message)...)
	if json.Unmarshal(searched.Data, &page) != nil || len(page.Items) == 0 {
		t.Fatal("log search omitted observed message")
	}
	human("logs-empty", "No log events on this page.", append(filtered, "--search", "igw-journey-absent-marker-61f87d")...)
	human("logs-help", "--since", "logs", "list", "--help")

	download := func(label, file string, args ...string) {
		t.Helper()
		path := filepath.Join(root, file)
		got := ok(label, append(args, "--out", path)...)
		if got.Artifact == nil {
			t.Fatal("missing artifact receipt")
		}
		observed := inspectOperationalArtifact(t, label, path)
		if observed.SHA256 != got.Artifact.SHA256 || observed.Bytes != got.Artifact.Bytes {
			t.Fatal("download receipt did not match file")
		}
	}
	download("backup-export", "gateway.gwbk", "backup", "export")
	download("logs-download", "system-logs.idb", "logs", "download")
	previewFile := filepath.Join(root, "preview.zip")
	ok("bundle-preview", "diagnostics", "bundle", "collect", "--out", previewFile, "--dry-run")
	if _, err := os.Stat(previewFile); !os.IsNotExist(err) {
		t.Fatal("bundle preview wrote an artifact")
	}
	download("bundle-collect", "diagnostics.zip", "diagnostics", "bundle", "collect", "--yes")
	ok("bundle-status", "diagnostics", "bundle", "status")
	s.completed = true
}

func journeyEnvironment(base []string, root string) []string {
	var env []string
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "IGNITION_GATEWAY_URL", "IGNITION_API_TOKEN", "XDG_CONFIG_HOME", "XDG_CACHE_HOME":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "XDG_CONFIG_HOME="+filepath.Join(root, "config"), "XDG_CACHE_HOME="+filepath.Join(root, "cache"))
}

func TestJourneyEnvironmentIsolatesConfiguration(t *testing.T) {
	got := journeyEnvironment([]string{"PATH=/bin", "IGNITION_API_TOKEN=private", "IGNITION_GATEWAY_URL=https://private.invalid", "XDG_CONFIG_HOME=/private", "XDG_CACHE_HOME=/private"}, "/fixture")
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "private") || !strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "XDG_CONFIG_HOME="+filepath.Join("/fixture", "config")) || !strings.Contains(joined, "XDG_CACHE_HOME="+filepath.Join("/fixture", "cache")) {
		t.Fatal("journey process inherited real configuration")
	}
}
