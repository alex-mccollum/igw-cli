// Package resource implements named configuration workflows over the typed
// execution core. It never parses CLI arguments or opens its own HTTP client.
package resource

import (
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

const prefix = "/data/api/v1/resources/"

var typePart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func ValidateType(id string) error {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || !typePart.MatchString(parts[0]) || !typePart.MatchString(parts[1]) {
		return result.Usage("resource type must be MODULE/TYPE from resource types")
	}
	return nil
}

type Type struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

func Types(c *catalog.Catalog) []Type {
	types := []Type{}
	for _, op := range c.Operations() {
		id, ok := strings.CutPrefix(op.Path, prefix+"type/")
		if op.Method == "GET" && ok && ValidateType(id) == nil {
			types = append(types, Type{ID: id, Summary: op.Summary})
		}
	}
	sort.Slice(types, func(i, j int) bool { return types[i].ID < types[j].ID })
	return types
}

type Runner interface {
	Run(execute.Request) result.Result
}

type Change struct {
	Action, Type, Name, Collection string
	Signature                      string
	Body                           []byte
	Yes, DryRun                    bool
}

// Validate rejects ambiguous bodies before acquiring a catalog or reading state.
// The returned fields preserve exact JSON numbers and do not include identity.
func (c Change) Validate() (map[string]json.RawMessage, error) {
	if err := ValidateType(c.Type); err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Collection) == "" {
		return nil, result.Usage("resource name and collection are required")
	}
	if c.Action != "create" && c.Action != "update" && c.Action != "delete" {
		return nil, result.Usage("unknown resource change")
	}
	if !c.Yes && !c.DryRun {
		return nil, result.Usage("resource changes require --yes; inspect --dry-run first")
	}
	if c.Action == "create" && c.Signature != "" {
		return nil, result.Usage("create requires absence, not a signature")
	}
	if c.Action != "create" && !c.DryRun && c.Signature == "" {
		return nil, result.Usage("update and delete require --if-signature from a reviewed get or dry-run")
	}
	if c.Action == "delete" {
		if len(c.Body) != 0 {
			return nil, result.Usage("delete does not accept a body")
		}
		return nil, nil
	}
	if err := jsonvalue.Validate(c.Body); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(c.Body, &fields) != nil || len(fields) == 0 {
		return nil, result.Usage("--body must be a nonempty JSON object of writable fields")
	}
	for key := range fields {
		switch key {
		case "description", "enabled", "config", "backupConfig":
		default:
			return nil, result.Usage("body accepts only description, enabled, config, and backupConfig; identity and signature use command arguments")
		}
	}
	return fields, nil
}

type Evidence struct {
	Action          string           `json:"action"`
	Type            string           `json:"type"`
	Name            string           `json:"name"`
	Collection      string           `json:"collection"`
	BeforeSignature string           `json:"beforeSignature,omitempty"`
	AfterSignature  string           `json:"afterSignature,omitempty"`
	ChangedFields   []string         `json:"changedFields"`
	State           string           `json:"state"`
	Request         *execute.Preview `json:"request,omitempty"`
}

// Apply sends at most one mutation. A 2xx response is insufficient: the
// Gateway's success flag, changed signature, and independent readback must
// agree. Configuration values are deliberately absent from workflow evidence.
func Apply(runner Runner, change Change) result.Result {
	fields, err := change.Validate()
	if err != nil {
		return result.Failure(err)
	}
	evidence := Evidence{Action: change.Action, Type: change.Type, Name: change.Name, Collection: change.Collection, ChangedFields: []string{}}
	get := execute.Request{Operation: "GET " + prefix + "find/" + change.Type + "/{name}", PathParams: map[string]string{"name": change.Name}, Query: url.Values{"collection": {change.Collection}}}
	beforeResult := runner.Run(get)
	before, exists, valid := state(beforeResult, change)
	if !valid {
		if !beforeResult.OK {
			return beforeResult
		}
		return problem(beforeResult, evidence, "resource_response", "Gateway returned an unverified resource identity", "failed")
	}
	if (change.Action == "create") == exists {
		return problem(beforeResult, evidence, "conflict", "resource existence differs from the requested change; inspect resource get", "failed")
	}
	if exists {
		evidence.BeforeSignature = stringField(before, "signature")
		if change.Signature != "" && evidence.BeforeSignature != change.Signature {
			return problem(beforeResult, evidence, "conflict", "resource signature differs from the reviewed version; inspect resource get before preparing a new change", "failed")
		}
	}
	for key, value := range fields {
		if !jsonvalue.Equivalent(value, before[key], false) {
			evidence.ChangedFields = append(evidence.ChangedFields, key)
		}
	}
	sort.Strings(evidence.ChangedFields)
	request := execute.Request{Yes: change.Yes, DryRun: change.DryRun}
	if change.Action == "delete" {
		request.Operation = "DELETE " + prefix + change.Type + "/{name}/{signature}"
		request.PathParams = map[string]string{"name": change.Name, "signature": evidence.BeforeSignature}
		request.Query = get.Query
	} else {
		body := make(map[string]json.RawMessage, len(fields)+3)
		for key, value := range fields {
			body[key] = value
		}
		body["name"], _ = json.Marshal(change.Name)
		body["collection"], _ = json.Marshal(change.Collection)
		request.Operation = "POST " + prefix + change.Type
		if change.Action == "update" {
			request.Operation = "PUT " + prefix + change.Type
			body["signature"], _ = json.Marshal(evidence.BeforeSignature)
		}
		request.Body, _ = json.Marshal([]any{body})
	}
	written := runner.Run(request)
	if change.DryRun {
		if written.OK {
			preview, ok := written.Data.(execute.Preview)
			if !ok {
				return problem(written, evidence, "resource_response", "request preview is unavailable", "failed")
			}
			evidence.Request, evidence.State = &preview, "not_changed"
			written.Data = evidence
		}
		return written
	}
	// Usage/schema errors happened before dispatch; authentication failures do
	// not establish a state transition. Keep their original exit-code contract.
	if !written.OK && written.Error != nil && (written.Error.Code == 2 || written.Error.Code == 6) {
		return written
	}
	var response struct {
		Success *bool                                                   `json:"success"`
		Changes []struct{ Name, Type, Collection, NewSignature string } `json:"changes"`
	}
	raw, _ := written.Data.(json.RawMessage)
	acknowledged := written.OK && jsonvalue.Validate(raw) == nil && json.Unmarshal(raw, &response) == nil && response.Success != nil && *response.Success
	newSignature := ""
	matches := 0
	for _, item := range response.Changes {
		if item.Name == change.Name && item.Type == change.Type && item.Collection == change.Collection {
			matches++
			if matches > 1 {
				acknowledged = false
				break
			}
			newSignature = item.NewSignature
		}
	}
	afterResult := runner.Run(get)
	after, afterExists, afterValid := state(afterResult, change)
	if !afterValid {
		evidence.State = "unknown"
		out := problem(written, evidence, "verification", "mutation was attempted but readback could not establish its outcome; inspect resource get before retrying", "uncertain")
		if afterResult.Error != nil {
			out.Error.Details = map[string]any{"mutation": out.Error.Details, "readback": afterResult.Error}
			if afterResult.Error.Code == 6 {
				out.Error.Code = 6
			}
		}
		return out
	}
	if afterExists {
		evidence.AfterSignature = stringField(after, "signature")
	}
	if change.Action == "delete" && !afterExists && acknowledged {
		evidence.State = "absent"
		return completed(written, evidence)
	}
	if change.Action != "delete" && acknowledged && afterExists && newSignature != "" && newSignature == evidence.AfterSignature && matchesFields(fields, before, after, change.Action == "update") {
		evidence.State = "matches_request"
		return completed(written, evidence)
	}
	if exists == afterExists && (!exists || evidence.BeforeSignature == evidence.AfterSignature && matchesFields(nil, before, after, true)) {
		evidence.State = "unchanged"
		if !acknowledged && written.Outcome != "uncertain" {
			return problem(written, evidence, "resource_rejected", "Gateway did not confirm the change; readback observed unchanged state", "failed")
		}
	} else {
		evidence.State = "changed_unverified"
	}
	return problem(written, evidence, "verification", "mutation outcome could not be verified; inspect resource get before deciding whether to retry", "uncertain")
}

func completed(out result.Result, evidence Evidence) result.Result {
	out.OK, out.Error, out.Outcome, out.Data = true, nil, "completed", evidence
	out.Meta.Verification = "verified"
	return out
}

func problem(out result.Result, evidence Evidence, kind, message, outcome string) result.Result {
	out.OK, out.Outcome, out.Data = false, outcome, evidence
	details := map[string]any{}
	if out.Error != nil {
		details["cause"] = out.Error.Kind
		details["response"] = out.Error.Details
	}
	out.Error = &result.Problem{Kind: kind, Message: message, Code: 7, Details: details}
	out.Meta.Verification = "not_verified"
	return out
}

func state(out result.Result, change Change) (map[string]json.RawMessage, bool, bool) {
	if !out.OK {
		if out.Error != nil && out.Error.Kind == "http" {
			b, _ := json.Marshal(out.Error.Details)
			var details struct {
				HTTPStatus int `json:"httpStatus"`
			}
			if json.Unmarshal(b, &details) == nil && details.HTTPStatus == 404 {
				return nil, false, true
			}
		}
		return nil, false, false
	}
	raw, ok := out.Data.(json.RawMessage)
	var object map[string]json.RawMessage
	if !ok || jsonvalue.Validate(raw) != nil || json.Unmarshal(raw, &object) != nil || stringField(object, "name") != change.Name || stringField(object, "collection") != change.Collection || stringField(object, "type") != change.Type || stringField(object, "signature") == "" {
		return nil, false, false
	}
	return object, true, true
}

func stringField(object map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}

func matchesFields(fields, before, after map[string]json.RawMessage, update bool) bool {
	for key, value := range fields {
		if !jsonvalue.Equivalent(value, after[key], true) {
			return false
		}
	}
	if update {
		for _, key := range []string{"description", "enabled", "config", "backupConfig"} {
			if _, supplied := fields[key]; !supplied && !jsonvalue.Equivalent(before[key], after[key], false) {
				return false
			}
		}
	}
	return true
}
