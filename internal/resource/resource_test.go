package resource

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type runFunc func(execute.Request) result.Result

func (f runFunc) Run(r execute.Request) result.Result { return f(r) }

func response(body string) result.Result {
	out := result.Success(json.RawMessage(body))
	out.Meta.HTTPStatus = 200
	return out
}

func current(signature, description string) result.Result {
	b, _ := json.Marshal(map[string]any{"type": "ignition/schedule", "name": "test", "collection": "core", "signature": signature, "description": description, "enabled": true, "config": map[string]any{"precise": json.Number("9007199254740993")}})
	return response(string(b))
}

func missing() result.Result {
	return result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 404}})
}

func TestUpdateRequiresAcknowledgementAndMatchingReadback(t *testing.T) {
	for _, tc := range []struct{ name, ack, signature, description, outcome, state string }{
		{"verified", `{"success":true,"changes":[{"name":"test","type":"ignition/schedule","collection":"core","newSignature":"next"}]}`, "next", "new", "completed", "matches_request"},
		{"wrong signature", `{"success":true,"changes":[{"name":"test","type":"ignition/schedule","collection":"core","newSignature":"different"}]}`, "next", "new", "uncertain", "changed_unverified"},
		{"wrong state", `{"success":true,"changes":[{"name":"test","type":"ignition/schedule","collection":"core","newSignature":"next"}]}`, "next", "wrong", "uncertain", "changed_unverified"},
		{"false success", `{"success":false,"problem":{"message":"private-secret"}}`, "before", "old", "failed", "unchanged"},
		{"no success field", `{}`, "next", "new", "uncertain", "changed_unverified"},
		{"wrong resource", `{"success":true,"changes":[{"name":"other","type":"ignition/schedule","collection":"core","newSignature":"next"}]}`, "next", "new", "uncertain", "changed_unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := runFunc(func(req execute.Request) result.Result {
				calls++
				switch calls {
				case 1:
					return current("before", "old")
				case 2:
					if req.Operation != "PUT /data/api/v1/resources/ignition/schedule" || !req.Yes || req.DryRun {
						t.Fatal("wrong mutation")
					}
					var body []map[string]json.RawMessage
					if json.Unmarshal(req.Body, &body) != nil || len(body) != 1 || len(body[0]) != 4 || stringField(body[0], "signature") != "before" {
						t.Fatal("signature or patch fields changed")
					}
					return response(tc.ack)
				case 3:
					return current(tc.signature, tc.description)
				default:
					t.Fatal("workflow replayed request")
					return result.Result{}
				}
			})
			got := Apply(runner, Change{Action: "update", Type: "ignition/schedule", Name: "test", Collection: "core", Body: []byte(`{"description":"new"}`), Signature: "before", Yes: true})
			b, _ := json.Marshal(got)
			if calls != 3 || got.Outcome != tc.outcome || got.OK != (tc.outcome == "completed") || got.Data.(Evidence).State != tc.state || strings.Contains(string(b), "private-secret") {
				t.Fatalf("incorrect evidence: %s", b)
			}
		})
	}
}

func TestPreviewAndStaleSignatureNeverMutate(t *testing.T) {
	for _, tc := range []struct {
		name, signature string
		preview         bool
		calls           int
		outcome         string
	}{
		{"preview", "", true, 2, "preview"},
		{"stale reviewed preview", "stale", true, 1, "failed"},
		{"stale execution", "stale", false, 1, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := runFunc(func(req execute.Request) result.Result {
				calls++
				if calls == 1 {
					return current("before", "old")
				}
				if !req.DryRun {
					t.Fatal("preview sent mutation")
				}
				out := result.Success(execute.Preview{Validation: "declared_schema"})
				out.Outcome = "preview"
				return out
			})
			got := Apply(runner, Change{Action: "update", Type: "ignition/schedule", Name: "test", Collection: "core", Body: []byte(`{"description":"private-secret"}`), Signature: tc.signature, DryRun: tc.preview, Yes: !tc.preview})
			b, _ := json.Marshal(got)
			if calls != tc.calls || got.Outcome != tc.outcome || strings.Contains(string(b), "private-secret") {
				t.Fatalf("incorrect preview: %s", b)
			}
		})
	}
}

func TestMutationFailureStillReadsStateWithoutReplay(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		calls := 0
		runner := runFunc(func(req execute.Request) result.Result {
			calls++
			if calls != 2 {
				return current("before", "old")
			}
			out := result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 500}})
			if uncertain {
				out.Outcome = "uncertain"
				out.Error.Kind = "transport"
			}
			return out
		})
		got := Apply(runner, Change{Action: "update", Type: "ignition/schedule", Name: "test", Collection: "core", Signature: "before", Yes: true, Body: []byte(`{"description":"new"}`)})
		if calls != 3 || got.OK || got.Data.(Evidence).State != "unchanged" || uncertain && got.Outcome != "uncertain" || !uncertain && got.Error.Kind == "conflict" {
			t.Fatalf("failure lost uncertainty or invented conflict: %+v", got)
		}
	}
}

func TestDeletionRequiresIndependentAbsence(t *testing.T) {
	for _, absent := range []bool{true, false} {
		calls := 0
		runner := runFunc(func(req execute.Request) result.Result {
			calls++
			if calls == 2 {
				if req.PathParams["signature"] != "before" || req.Query.Get("confirm") != "" {
					t.Fatal("delete lost precondition or forced referenced deletion")
				}
				return response(`{"success":true}`)
			}
			if calls == 3 && absent {
				return missing()
			}
			return current("before", "old")
		})
		got := Apply(runner, Change{Action: "delete", Type: "ignition/schedule", Name: "test", Collection: "core", Signature: "before", Yes: true})
		if calls != 3 || got.OK != absent {
			t.Fatal("deletion inferred success without absence")
		}
	}
}

func TestCreateRefusesExistingResource(t *testing.T) {
	calls := 0
	got := Apply(runFunc(func(req execute.Request) result.Result { calls++; return current("before", "old") }), Change{Action: "create", Type: "ignition/schedule", Name: "test", Collection: "core", Yes: true, Body: []byte(`{"enabled":true}`)})
	if calls != 1 || got.OK || got.Error.Kind != "conflict" {
		t.Fatal("create attempted overwrite")
	}
}

func TestReadbackAuthFailurePreservesUncertainOutcome(t *testing.T) {
	calls := 0
	got := Apply(runFunc(func(req execute.Request) result.Result {
		calls++
		switch calls {
		case 1:
			return current("before", "old")
		case 2:
			return response(`{"success":true}`)
		default:
			return result.Failure(&result.Problem{Kind: "auth", Code: 6, Message: "permission denied"})
		}
	}), Change{Action: "delete", Type: "ignition/schedule", Name: "test", Collection: "core", Signature: "before", Yes: true})
	if calls != 3 || got.OK || got.Outcome != "uncertain" || got.Error.Code != 6 || got.Data.(Evidence).State != "unknown" {
		t.Fatalf("readback failure contract: %+v", got)
	}
}

func TestChangeRejectsAmbiguousOrUnconfirmedInput(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{}`, `{"name":"other"}`, `{"config":{"x":1,"x":2}}`, `{"enabled":true,"enabled":false}`, `{"enabled":true} {}`} {
		_, err := (Change{Action: "create", Type: "ignition/schedule", Name: "test", Collection: "core", Yes: true, Body: []byte(body)}).Validate()
		if err == nil {
			t.Fatalf("accepted body: %s", body)
		}
	}
	for _, change := range []Change{
		{Action: "update", Type: "ignition/schedule", Name: "test", Collection: "core", Yes: true, Body: []byte(`{"enabled":true}`)},
		{Action: "delete", Type: "ignition/schedule", Name: "test", Collection: "core", Signature: "before"},
	} {
		if _, err := change.Validate(); err == nil {
			t.Fatal("accepted unconfirmed or unreviewed change")
		}
	}
}

func TestVerificationPreservesExactNumbersAndOmittedConfig(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{
		{`9007199254740993`, `9007199254740992`, false},
		{`1.2300e1000000`, `123e999998`, true},
		{`-0`, `0.0`, true},
		{`1e99999999999999999999999999`, `1e99999999999999999999999998`, false},
	} {
		if equivalent([]byte(tc.a), []byte(tc.b), false) != tc.equal {
			t.Fatal("numeric comparison lost precision")
		}
	}
	fields := map[string]json.RawMessage{"description": json.RawMessage(`"new"`)}
	before := map[string]json.RawMessage{"config": json.RawMessage(`{"precise":9007199254740993}`)}
	after := map[string]json.RawMessage{"description": json.RawMessage(`"new"`), "config": json.RawMessage(`{"precise":9007199254740992}`)}
	if matchesFields(fields, before, after, true) {
		t.Fatal("verification ignored lost configuration")
	}
	if !reflect.DeepEqual(fields, map[string]json.RawMessage{"description": json.RawMessage(`"new"`)}) {
		t.Fatal("verification rewrote submitted fields")
	}
}
