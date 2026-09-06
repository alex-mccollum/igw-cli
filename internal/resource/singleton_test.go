package resource

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func singletonState(signature, description string) result.Result {
	raw, _ := json.Marshal(map[string]any{"type": "test/settings", "collection": "core", "signature": signature, "name": "server-assigned", "description": description, "enabled": true, "config": map[string]any{"exact": json.Number("9007199254740993")}})
	return response(string(raw))
}

func TestSingletonChangesUseTypeCollectionAndSignature(t *testing.T) {
	for _, action := range []string{"create", "update", "delete"} {
		for _, preview := range []bool{true, false} {
			t.Run(action+map[bool]string{true: "/preview", false: "/execute"}[preview], func(t *testing.T) {
				calls := 0
				runner := runFunc(func(req execute.Request) result.Result {
					calls++
					if calls == 1 || calls == 3 {
						if req.Operation != "GET /data/api/v1/resources/singleton/test/settings" || req.Query.Encode() != "collection=core&defaultIfUndefined=false" || len(req.PathParams) != 0 {
							t.Fatal("singleton read inferred a name or requested defaults")
						}
						if calls == 1 && action == "create" || calls == 3 && action == "delete" {
							return missing()
						}
						if calls == 1 {
							return singletonState("before", "old")
						}
						return singletonState("after", "new")
					}
					if calls != 2 || req.DryRun != preview || req.Yes == preview {
						t.Fatal("unexpected dispatch or replay")
					}
					if action == "delete" {
						if req.Operation != "DELETE /data/api/v1/resources/test/settings/{signature}" || len(req.PathParams) != 1 || req.PathParams["signature"] != "before" || req.Query.Encode() != "collection=core" || req.Body != nil {
							t.Fatal("singleton delete identity or query changed")
						}
					} else {
						method := "POST"
						if action == "update" {
							method = "PUT"
						}
						var body []map[string]json.RawMessage
						if req.Operation != method+" /data/api/v1/resources/test/settings" || json.Unmarshal(req.Body, &body) != nil || len(body) != 1 || stringField(body[0], "collection") != "core" || stringField(body[0], "description") != "new" {
							t.Fatal("singleton mutation body changed")
						}
						if _, exists := body[0]["name"]; exists {
							t.Fatal("singleton body contains an invented name")
						}
						if action == "update" && stringField(body[0], "signature") != "before" {
							t.Fatal("missing observed precondition")
						}
					}
					if preview {
						out := result.Success(execute.Preview{})
						out.Outcome = "preview"
						return out
					}
					return response(`{"success":true,"changes":[{"type":"test/settings","collection":"core","name":"optional-alias","newSignature":"after"}]}`)
				})
				change := Change{Action: action, Type: "test/settings", Collection: "core", Singleton: true, Yes: !preview, DryRun: preview}
				if action != "delete" {
					change.Body = []byte(`{"description":"new"}`)
				}
				if action != "create" {
					change.Signature = "before"
				}
				got := Apply(runner, change)
				if !got.OK || !got.Data.(Evidence).Singleton || got.Data.(Evidence).Name != "" {
					t.Fatalf("singleton outcome: %+v", got)
				}
				if preview && (calls != 2 || got.Outcome != "preview") || !preview && (calls != 3 || got.Outcome != "completed" || got.Meta.Verification != "verified") {
					t.Fatal("singleton completion evidence incorrect")
				}
			})
		}
	}
}

func TestSingletonRefusalsAndUncertainOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, before, ack string
		wantCalls         int
		outcome           string
	}{
		{"wrong collection", `{"type":"test/settings","collection":"other","signature":"before"}`, "", 1, "failed"},
		{"wrong type", `{"type":"test/other","collection":"core","signature":"before"}`, "", 1, "failed"},
		{"stale signature", `{"type":"test/settings","collection":"core","signature":"stale"}`, "", 1, "failed"},
		{"missing signature", `{"type":"test/settings","collection":"core"}`, "", 1, "failed"},
		{"duplicate acknowledgement", "", `{"success":true,"changes":[{"type":"test/settings","collection":"core","newSignature":"after"},{"type":"test/settings","collection":"core","newSignature":"after"}]}`, 3, "uncertain"},
		{"wrong acknowledgement", "", `{"success":true,"changes":[{"type":"test/other","collection":"core","newSignature":"after"}]}`, 3, "uncertain"},
		{"unacknowledged write", "", `{"success":false}`, 3, "uncertain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got := Apply(runFunc(func(req execute.Request) result.Result {
				calls++
				switch calls {
				case 1:
					if tc.before != "" {
						return response(tc.before)
					}
					return singletonState("before", "old")
				case 2:
					return response(tc.ack)
				case 3:
					return singletonState("after", "new")
				}
				t.Fatal("write replayed")
				return result.Result{}
			}), Change{Action: "update", Type: "test/settings", Singleton: true, Collection: "core", Signature: "before", Yes: true, Body: []byte(`{"description":"new"}`)})
			if got.OK || got.Outcome != tc.outcome || calls != tc.wantCalls {
				t.Fatalf("incorrect singleton refusal: %+v calls=%d", got, calls)
			}
		})
	}
	for _, name := range []string{"", "invented"} {
		change := Change{Action: "update", Type: "test/settings", Name: name, Singleton: true, Collection: "core", Body: []byte(`{"description":"new"}`), Yes: true}
		if _, err := change.Validate(); err == nil {
			t.Fatal("unreviewed or named singleton mutation accepted")
		}
	}
}

func TestResourceTypesExposeSingletonReadRoutes(t *testing.T) {
	const raw = `{"openapi":"3.1.0","info":{"title":"Synthetic resource discovery","version":"test"},"paths":{"/data/api/v1/resources/type/test/settings":{"get":{"responses":{"200":{"description":"OK"}}}},"/data/api/v1/resources/singleton/test/settings":{"get":{"responses":{"200":{"description":"OK"}}}},"/data/api/v1/resources/type/test/items":{"get":{"responses":{"200":{"description":"OK"}}}}}}`
	c, err := catalog.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	types := Types(c)
	if len(types) != 2 || types[0].ID != "test/items" || types[0].Singleton || types[1].ID != "test/settings" || !types[1].Singleton {
		t.Fatalf("incorrect type modes: %+v", types)
	}
	for _, singleton := range []bool{false, true} {
		name := ""
		if !singleton {
			name = "unit/+%2F 日本"
		}
		get, err := ReadRequest("test/settings", name, "core", singleton)
		if err != nil || singleton != strings.Contains(get.Operation, "/singleton/") {
			t.Fatalf("read mode: %+v %v", get, err)
		}
	}
}
