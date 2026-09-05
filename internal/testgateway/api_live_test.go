package testgateway_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/nextcli"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

// This test is compiled before starting Docker workloads. All HTTP requests
// after bootstrap use the real CLI with an API token and no browser cookies.
func TestLiveAPIResourceContract(t *testing.T) {
	image := os.Getenv("IGW_ACCEPTANCE_TEST_IMAGE")
	if image == "" {
		t.Skip("requires an explicit pinned image and guarded live-test invocation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	started := time.Now().UTC()
	s, err := testgateway.Start(ctx, testgateway.Config{Image: image, Docker: os.Getenv("IGW_CAPTURE_TEST_DOCKER")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Errorf("owned container cleanup failed: %v", err)
		}
	}()
	t.Log("waiting for disposable Gateway readiness")
	if _, err := s.WaitOpenAPI(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("provisioning ephemeral API token")
	token, err := s.ProvisionAPIToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	type envelope struct {
		OK      bool            `json:"ok"`
		Outcome string          `json:"outcome"`
		Data    json.RawMessage `json:"data"`
		Error   *result.Problem `json:"error"`
		Meta    result.Metadata `json:"meta"`
	}
	type check struct {
		Name       string `json:"name"`
		Outcome    string `json:"outcome"`
		HTTPStatus int    `json:"httpStatus,omitempty"`
	}
	var checks []check
	_, secret, _ := strings.Cut(token, ":")
	runWithToken := func(name, credential string, args ...string) envelope {
		t.Helper()
		var out, stderr bytes.Buffer
		app := nextcli.App{In: strings.NewReader(""), Out: &out, Err: &stderr, CacheDir: cache, HTTP: s.HTTPClient(), Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) {
			return config.File{GatewayURL: s.URL, Token: credential}, nil
		}}
		err := app.Run(ctx, append(args, "--json", "--timeout", "90s"))
		if strings.Contains(out.String(), token) || strings.Contains(stderr.String(), token) || (secret != "" && (strings.Contains(out.String(), secret) || strings.Contains(stderr.String(), secret))) {
			t.Fatal("API credential leaked through CLI output")
		}
		var got envelope
		if json.Unmarshal(out.Bytes(), &got) != nil || (err == nil) != got.OK {
			t.Fatalf("%s: invalid CLI result envelope", name)
		}
		status := got.Meta.HTTPStatus
		if got.Error != nil {
			b, _ := json.Marshal(got.Error.Details)
			var details struct {
				HTTPStatus int `json:"httpStatus"`
			}
			_ = json.Unmarshal(b, &details)
			status = details.HTTPStatus
		}
		checks = append(checks, check{Name: name, Outcome: got.Outcome, HTTPStatus: status})
		t.Logf("%s: %s HTTP=%d", name, got.Outcome, status)
		return got
	}
	run := func(name string, args ...string) envelope { return runWithToken(name, token, args...) }
	sync := run("authenticated-catalog", "spec", "sync")
	if !sync.OK || sync.Meta.Catalog == nil || sync.Meta.Catalog.SourceKind != "gateway" {
		t.Fatal("API token could not acquire a target Gateway catalog")
	}
	for _, tc := range []struct{ name, credential string }{{"anonymous-denied", ""}, {"bare-key-denied", secret}} {
		got := runWithToken(tc.name, tc.credential, "spec", "sync")
		status := checks[len(checks)-1].HTTPStatus
		if got.OK || got.Error == nil || got.Error.Code != 6 || (status != 401 && status != 403) {
			t.Fatal("scoped bootstrap granted unauthenticated access")
		}
	}
	const resource = "/data/api/v1/resources/ignition/schedule"
	const find = "GET /data/api/v1/resources/find/ignition/schedule/{name}"
	const name = "igw-acceptance"
	const createBody = `[{"name":"igw-acceptance","collection":"core","description":"created by acceptance","enabled":true,"config":{"profile":{"type":"basic schedule"},"settings":{"allDays":true,"allDayTime":"01:00-02:00"}}}]`
	get := func(step string) envelope {
		return run(step, "api", "request", find, "--path-param", "name="+name, "--query", "collection=core")
	}
	absent := func(step string) {
		t.Helper()
		got := get(step)
		last := checks[len(checks)-1]
		if got.OK || last.HTTPStatus != 404 {
			t.Fatalf("%s: expected independently observed absence", step)
		}
	}
	preview := run("preview-create", "api", "request", "POST "+resource, "--body", createBody, "--dry-run")
	if !preview.OK || preview.Outcome != "preview" {
		t.Fatal("create preview failed")
	}
	absent("absence-after-preview")
	confirmed := func(step string, got envelope) {
		t.Helper()
		var response struct {
			Success bool `json:"success"`
		}
		if !got.OK || got.Outcome != "accepted" || json.Unmarshal(got.Data, &response) != nil || !response.Success {
			t.Fatalf("%s: Gateway did not confirm the resource mutation", step)
		}
	}
	confirmed("create", run("create", "api", "request", "POST "+resource, "--body", createBody, "--yes"))
	type state struct {
		Name, Collection, Signature, Description string
		Config                                   json.RawMessage
	}
	readState := func(step string) state {
		t.Helper()
		got := get(step)
		var state state
		if !got.OK || json.Unmarshal(got.Data, &state) != nil || state.Name != name || state.Collection != "core" || state.Signature == "" {
			t.Fatalf("%s: invalid resource identity or signature", step)
		}
		return state
	}
	created := readState("verify-created")
	if created.Description != "created by acceptance" {
		t.Fatal("created state differs from requested description")
	}
	update := func(signature, description string) string {
		b, _ := json.Marshal([]any{map[string]string{"name": name, "collection": "core", "signature": signature, "description": description}})
		return string(b)
	}
	confirmed("update", run("update", "api", "request", "PUT "+resource, "--body", update(created.Signature, "updated by acceptance"), "--yes"))
	updated := readState("verify-updated")
	if updated.Description != "updated by acceptance" || updated.Signature == created.Signature || !bytes.Equal(updated.Config, created.Config) {
		t.Fatal("partial update changed omitted configuration or lacked a new signature")
	}
	stale := run("stale-signature", "api", "request", "PUT "+resource, "--body", update(created.Signature, "must not apply"), "--yes")
	var staleResponse struct {
		Success bool `json:"success"`
	}
	_ = json.Unmarshal(stale.Data, &staleResponse)
	if stale.OK && staleResponse.Success {
		t.Fatal("stale signature was accepted")
	}
	afterConflict := readState("verify-conflict-unchanged")
	if afterConflict.Signature != updated.Signature || afterConflict.Description != updated.Description {
		t.Fatal("stale mutation changed current state")
	}
	confirmed("delete", run("delete", "api", "request", "DELETE "+resource+"/{name}/{signature}", "--path-param", "name="+name, "--path-param", "signature="+updated.Signature, "--query", "collection=core", "--yes"))
	absent("verify-deleted")
	for _, args := range [][]string{{"types"}, {"describe", "ignition/schedule"}, {"list", "ignition/schedule", "--limit", "10"}} {
		if !run("resource-"+args[0], append([]string{"resource"}, args...)...).OK {
			t.Fatal("resource discovery failed")
		}
	}
	const workflowBody = `{"description":"created by workflow","enabled":true,"config":{"profile":{"type":"basic schedule"},"settings":{"allDays":true,"allDayTime":"01:00-02:00"}}}`
	if got := run("resource-preview-create", "resource", "create", "ignition/schedule", name, "--body", workflowBody, "--dry-run"); !got.OK || got.Outcome != "preview" {
		t.Fatal("resource preview failed")
	}
	absent("resource-preview-absence")
	completed := func(step string, got envelope) string {
		t.Helper()
		var evidence struct{ AfterSignature string }
		_ = json.Unmarshal(got.Data, &evidence)
		if !got.OK || got.Outcome != "completed" || got.Meta.Verification != "verified" {
			// Error kinds and state evidence contain no configuration values.
			kind := ""
			if got.Error != nil {
				kind = got.Error.Kind
			}
			t.Fatalf("%s: workflow outcome=%s error=%s data=%s", step, got.Outcome, kind, got.Data)
		}
		return evidence.AfterSignature
	}
	createdSignature := completed("resource-create", run("resource-create", "resource", "create", "ignition/schedule", name, "--body", workflowBody, "--yes"))
	if createdSignature == "" {
		t.Fatal("creation omitted verified signature")
	}
	if got := run("resource-get", "resource", "get", "ignition/schedule", name); !got.OK {
		t.Fatal("resource get failed")
	}
	if got := run("resource-duplicate-create", "resource", "create", "ignition/schedule", name, "--body", workflowBody, "--yes"); got.OK || got.Error == nil || got.Error.Kind != "conflict" {
		t.Fatal("resource create did not enforce absence")
	}
	if got := run("resource-preview-update", "resource", "update", "ignition/schedule", name, "--body", `{"description":"updated by workflow"}`, "--dry-run"); !got.OK || got.Outcome != "preview" {
		t.Fatal("update preview failed")
	}
	updatedSignature := completed("resource-update", run("resource-update", "resource", "update", "ignition/schedule", name, "--body", `{"description":"updated by workflow"}`, "--if-signature", createdSignature, "--yes"))
	if updatedSignature == "" || updatedSignature == createdSignature {
		t.Fatal("update omitted new verified signature")
	}
	if got := run("resource-stale-update", "resource", "update", "ignition/schedule", name, "--body", `{"description":"must not apply"}`, "--if-signature", createdSignature, "--yes"); got.OK || got.Error == nil || got.Error.Kind != "conflict" {
		t.Fatal("workflow discarded reviewed signature")
	}
	if got := run("resource-preview-delete", "resource", "delete", "ignition/schedule", name, "--dry-run"); !got.OK || got.Outcome != "preview" {
		t.Fatal("delete preview failed")
	}
	completed("resource-delete", run("resource-delete", "resource", "delete", "ignition/schedule", name, "--if-signature", updatedSignature, "--yes"))
	absent("resource-verified-deleted")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("IGW_ACCEPTANCE_EVIDENCE"); path != "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(executable)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.New()
		_, hashErr := io.Copy(hash, f)
		closeErr := f.Close()
		if hashErr != nil || closeErr != nil {
			t.Fatal("could not hash acceptance binary")
		}
		evidence := struct {
			Version          int               `json:"version"`
			Kind             string            `json:"kind"`
			Image            string            `json:"image"`
			ImageID          string            `json:"imageId"`
			Platform         string            `json:"platform"`
			GatewayVersion   string            `json:"gatewayVersion"`
			TestBinarySHA256 string            `json:"testBinarySha256"`
			StartedAt        time.Time         `json:"startedAt"`
			FinishedAt       time.Time         `json:"finishedAt"`
			Catalog          *catalog.Metadata `json:"catalog"`
			Checks           []check           `json:"checks"`
			Cleanup          bool              `json:"cleanup"`
			Passed           bool              `json:"passed"`
		}{2, "resource-workflows", image, s.ImageID, s.Platform, s.GatewayVersion, hex.EncodeToString(hash.Sum(nil)), started, time.Now().UTC(), sync.Meta.Catalog, checks, true, true}
		b, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		w, err := artifact.New(path, false)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Abort()
		if _, err := w.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}
