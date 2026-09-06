package testgateway_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/resource"
)

type inputResourceEvidence struct {
	Action                string                       `json:"action"`
	Type                  string                       `json:"type"`
	Singleton             bool                         `json:"singleton"`
	State                 string                       `json:"state"`
	BeforeSignatureSHA256 string                       `json:"beforeSignatureSha256,omitempty"`
	AfterSignatureSHA256  string                       `json:"afterSignatureSha256,omitempty"`
	ChangedFields         []string                     `json:"changedFields"`
	Request               *execute.Preview             `json:"request,omitempty"`
	Checks                *resource.VerificationChecks `json:"checks,omitempty"`
}

func resourceEvidence(raw json.RawMessage) *inputResourceEvidence {
	var e resource.Evidence
	if json.Unmarshal(raw, &e) != nil || e.Action == "" || e.Type == "" {
		return nil
	}
	out := &inputResourceEvidence{Action: e.Action, Type: e.Type, Singleton: e.Singleton, State: e.State, ChangedFields: e.ChangedFields, Request: e.Request, Checks: e.Checks}
	if e.BeforeSignature != "" {
		out.BeforeSignatureSHA256 = inputDigest([]byte(e.BeforeSignature))
	}
	if e.AfterSignature != "" {
		out.AfterSignatureSHA256 = inputDigest([]byte(e.AfterSignature))
	}
	return out
}

// This changes only the translations definition in a fresh owned Gateway.
// Delete/create deliberately exercise stored absence independently of whether
// the image initially supplies that singleton. No host lifecycle controls run.
func TestLiveSingletonResources(t *testing.T) {
	s := beginObservedSuite(t, "IGW_SINGLETON_EVIDENCE_DIR", "singleton-resources", "singleton.json")
	const kind = "ignition/translations"
	const read = "GET /data/api/v1/resources/singleton/ignition/translations"
	const initial = `{"description":"igw singleton qualification","enabled":true,"config":{"caseInsensitive":false,"ignoreWhitespace":false,"ignorePunctuation":false,"ignoreTags":false,"terms":{}}}`
	const changed = `{"description":"igw singleton verified update"}`
	s.refused(s.run("confirmation", nil, "resource", "update", kind, "--body", changed))
	s.refused(s.run("review-required", nil, "resource", "update", kind, "--body", changed, "--yes"))
	s.refused(s.run("named-route-refusal", nil, "resource", "get", kind, "invented-name"))
	types := s.run("types", nil, "resource", "types")
	s.require(types, 0)
	var discovered []resource.Type
	if json.Unmarshal(types.Data, &discovered) != nil {
		t.Fatal("invalid resource type discovery")
	}
	found := false
	for _, item := range discovered {
		if item.ID == kind && item.Singleton {
			found = true
		}
	}
	if !found {
		t.Fatal("translations singleton was not advertised")
	}
	get := func(label string) transferResult {
		return s.run(label, nil, "api", "request", read, "--query", "collection=core", "--query", "defaultIfUndefined=false")
	}
	state := func(got transferResult) map[string]json.RawMessage {
		t.Helper()
		s.require(got, 1)
		var fields map[string]json.RawMessage
		var id struct{ Type, Collection, Signature string }
		if jsonvalue.Validate(got.Data) != nil || json.Unmarshal(got.Data, &fields) != nil || json.Unmarshal(got.Data, &id) != nil || id.Type != kind || id.Collection != "core" || id.Signature == "" {
			t.Fatal("singleton identity unavailable")
		}
		return fields
	}
	signature := func(fields map[string]json.RawMessage) string {
		var v string
		_ = json.Unmarshal(fields["signature"], &v)
		return v
	}
	noWrites := func() {
		for _, wire := range s.last().Wire {
			if wire.Method != "GET" {
				t.Fatal("read or preview dispatched a mutation")
			}
		}
	}
	completed := func(got transferResult, action string) {
		t.Helper()
		s.require(got, 3)
		e := s.last().Resource
		if got.Outcome != "completed" || got.Meta.Verification != "verified" || e == nil || !e.Singleton || e.Action != action {
			t.Fatal("singleton workflow not verified")
		}
		writes := 0
		for _, wire := range s.last().Wire {
			if wire.Method != "GET" {
				writes++
			}
		}
		if writes != 1 || s.last().CatalogRequests != 1 {
			t.Fatal("singleton workflow did not use one fresh catalog and one mutation")
		}
	}
	preview := func(got transferResult, action string) {
		t.Helper()
		s.require(got, 1)
		noWrites()
		e := s.last().Resource
		if got.Outcome != "preview" || e == nil || !e.Singleton || e.Action != action || e.Request == nil || !e.Request.Mutating {
			t.Fatal("singleton preview unavailable")
		}
	}
	before := s.run("get-before", nil, "resource", "get", kind)
	if !before.OK && inputHTTPStatus(before) == 404 && s.last().OperationRequests == 1 {
		preview(s.run("initial-create-preview", nil, "resource", "create", kind, "--body", initial, "--dry-run"), "create")
		completed(s.run("initial-create", nil, "resource", "create", kind, "--body", initial, "--yes"), "create")
		before = get("created-baseline")
	}
	baseline := state(before)
	preview(s.run("update-preview", nil, "resource", "update", kind, "--body", changed, "--dry-run"), "update")
	afterPreview := state(get("update-preview-unchanged"))
	for _, field := range []string{"signature", "description", "enabled", "config", "backupConfig"} {
		if !jsonvalue.Equivalent(baseline[field], afterPreview[field], false) {
			t.Fatal("preview changed stored configuration")
		}
	}
	completed(s.run("update", nil, "resource", "update", kind, "--body", changed, "--if-signature", signature(baseline), "--yes"), "update")
	updated := state(get("updated-readback"))
	if signature(updated) == signature(baseline) || !jsonvalue.Equivalent(updated["description"], []byte(`"igw singleton verified update"`), false) {
		t.Fatal("independent singleton update readback mismatch")
	}
	stale := s.run("stale-review", nil, "resource", "update", kind, "--body", initial, "--if-signature", signature(baseline), "--yes")
	if stale.OK || stale.Error == nil || stale.Error.Kind != "conflict" || stale.Error.Code != 7 || s.last().OperationRequests != 1 {
		t.Fatal("stale singleton review was not refused")
	}
	noWrites()
	preview(s.run("delete-preview", nil, "resource", "delete", kind, "--dry-run"), "delete")
	completed(s.run("delete", nil, "resource", "delete", kind, "--if-signature", signature(updated), "--yes"), "delete")
	absent := get("deleted-readback")
	if absent.OK || inputHTTPStatus(absent) != 404 || s.last().OperationRequests != 1 {
		t.Fatal("singleton deletion did not establish absence")
	}
	preview(s.run("create-preview", nil, "resource", "create", kind, "--body", initial, "--dry-run"), "create")
	created := s.run("create", nil, "resource", "create", kind, "--body", initial, "--yes")
	if !created.OK {
		// Inspect once without replaying. Record only comparisons for authored
		// fixture fields; never dump the resource's configuration or credentials.
		observed := state(get("uncertain-create-readback"))
		var expected map[string]json.RawMessage
		_ = json.Unmarshal([]byte(initial), &expected)
		for _, key := range []string{"description", "enabled", "config"} {
			t.Logf("recreation comparison %s: matches=%t present=%t", key, jsonvalue.Equivalent(expected[key], observed[key], true), len(observed[key]) > 0)
		}
		var wantConfig, gotConfig map[string]json.RawMessage
		_ = json.Unmarshal(expected["config"], &wantConfig)
		_ = json.Unmarshal(observed["config"], &gotConfig)
		for _, key := range []string{"caseInsensitive", "ignoreWhitespace", "ignorePunctuation", "ignoreTags", "terms"} {
			t.Logf("recreation comparison config.%s: matches=%t present=%t", key, jsonvalue.Equivalent(wantConfig[key], gotConfig[key], true), len(gotConfig[key]) > 0)
		}
		t.Fatal("singleton recreation remained unverified; readback retained without another mutation")
	}
	completed(created, "create")
	recreated := state(get("recreated-readback"))
	var submitted map[string]json.RawMessage
	_ = json.Unmarshal([]byte(initial), &submitted)
	for field, value := range submitted {
		if !jsonvalue.Equivalent(value, recreated[field], true) {
			t.Fatal("independent singleton create readback mismatch")
		}
	}
	duplicate := s.run("duplicate-create", nil, "resource", "create", kind, "--body", initial, "--yes")
	if duplicate.OK || duplicate.Error == nil || duplicate.Error.Kind != "conflict" || s.last().OperationRequests != 1 {
		t.Fatal("duplicate singleton creation was not refused")
	}
	noWrites()
	s.completed = true
}

func TestResourceEvidenceOmitsInstanceValues(t *testing.T) {
	e := resource.Evidence{Action: "update", Type: "ignition/translations", Name: "private-name", Collection: "private-collection", Singleton: true, BeforeSignature: "private-before", AfterSignature: "private-after", ChangedFields: []string{"config"}, State: "matches_request"}
	raw, _ := json.Marshal(e)
	got := resourceEvidence(raw)
	if got == nil || got.BeforeSignatureSHA256 != inputDigest([]byte(e.BeforeSignature)) || got.AfterSignatureSHA256 != inputDigest([]byte(e.AfterSignature)) || !got.Singleton {
		t.Fatal("resource evidence lost signatures")
	}
	raw, _ = json.Marshal(got)
	if strings.Contains(string(raw), "private-") {
		t.Fatal("resource evidence exposed instance values")
	}
}
