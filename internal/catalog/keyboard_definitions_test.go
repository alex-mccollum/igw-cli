package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// The layout schema is extracted from the real 8.3.0 document. The surrounding
// operations are reduced fixtures, retaining the four observed schema positions.
func keyboardFixture(t *testing.T, kind string) map[string]any {
	t.Helper()
	b, err := os.ReadFile("testdata/legacy-keyboard-config.json")
	if err != nil {
		t.Fatal(err)
	}
	var primary, backup map[string]any
	if json.Unmarshal(b, &primary) != nil || json.Unmarshal(b, &backup) != nil {
		t.Fatal("invalid keyboard schema fixture")
	}
	properties := map[string]any{"name": map[string]any{"type": "string"}, "signature": map[string]any{"type": "string"}, "config": primary, "backupConfig": backup}
	resource := map[string]any{"type": "object", "properties": properties, "required": []any{"name"}}
	op := map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}}
	path, method := "/data/api/v1/resources/ignition/keyboard_layout", strings.ToLower(kind)
	media := func(schema any) map[string]any {
		return map[string]any{"application/json": map[string]any{"schema": schema}}
	}
	switch kind {
	case "POST", "PUT":
		if kind == "PUT" {
			resource["required"] = []any{"name", "signature"}
		}
		op["requestBody"] = map[string]any{"required": true, "content": media(map[string]any{"type": "array", "items": resource})}
	case "find":
		path, method = "/data/api/v1/resources/find/ignition/keyboard_layout/{name}", "get"
		op["parameters"] = []any{map[string]any{"name": "name", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}
		op["responses"] = map[string]any{"200": map[string]any{"description": "OK", "content": media(resource)}}
	case "list":
		path, method = "/data/api/v1/resources/list/ignition/keyboard_layout", "get"
		op["responses"] = map[string]any{"200": map[string]any{"description": "OK", "content": media(map[string]any{"type": "object", "properties": map[string]any{"items": map[string]any{"type": "array", "items": resource}}})}}
	default:
		t.Fatal("unknown fixture kind")
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Ignition HTTP API", "version": "1.0.0", "license": map[string]any{"name": "Inductive Automation EULA", "url": "/res/sys/license.html"}}, "paths": map[string]any{path: map[string]any{method: op}}}
}

func TestKeyboardDefinitionsPreserveDiscoveryAndValueConstraints(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"POST", "PUT", "find", "list"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(keyboardFixture(t, kind))
			c, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if !bytes.Equal(c.Raw(), raw) || c.RawHash() != digest(raw) || c.Compatibility().Rules["keyboard-local-definitions"] != 2 {
				t.Fatal("vendor evidence or correction count changed")
			}
			op := c.Operations()[0]
			d, err := c.Describe(op.Key)
			if err != nil || len(d.Adjustments) != 2 || len(d.Gaps) != 1 || !bytes.Contains(d.Operation.Definition, []byte(`"$ref":"#/$defs/key"`)) {
				t.Fatalf("original refs or correction description lost: %+v %v", d, err)
			}
			if op.Method == "GET" {
				req, _ := http.NewRequest("GET", "http://gateway.test"+strings.ReplaceAll(op.Path, "{name}", "fixture"), nil)
				if issues, err := c.Validate(op.Key, req); err != nil || len(issues) != 0 {
					t.Fatalf("read contract changed: %v %v", issues, err)
				}
				return
			}
			for _, tc := range []struct {
				rows  string
				valid bool
			}{
				{`[[{"lowercase":{"character":"e","accents":["é"]},"uppercase":"E"},"SHIFT"]]`, true},
				{`[[{"lowercase":1,"uppercase":{"character":"E","alias":null}}]]`, true},
				{`[[{"lowercase":false}]]`, false},
				{`[[{"uppercase":"E"}]]`, false},
				{`[[{"lowercase":"e","extra":true}]]`, false},
				{`[[{"lowercase":{"character":"e","accents":[123]}}]]`, false},
				{`[[{"lowercase":{"alias":"e"}}]]`, false},
				{`[["UNKNOWN_KEY"]]`, false},
			} {
				for _, field := range []string{"config", "backupConfig"} {
					body := `[{"name":"fixture","signature":"observed","` + field + `":{"id":"layout-id","name":"TestLayout","title":"Test layout","alias":"EN","rows":` + tc.rows + `}}]`
					req, _ := http.NewRequest(op.Method, "http://gateway.test"+op.Path, strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					issues, err := c.Validate(op.Key, req)
					if err != nil || (len(issues) == 0) != tc.valid {
						t.Fatalf("%s %s constraints: %v %v", field, tc.rows, issues, err)
					}
				}
			}
		})
	}
}

func TestKeyboardDefinitionsRejectUnreviewedScopesAtomically(t *testing.T) {
	t.Parallel()
	for _, change := range []func(map[string]any, map[string]any, map[string]any){
		func(root, op, config map[string]any) { object(root["info"])["title"] = "Other API" },
		func(root, op, config map[string]any) { root["jsonSchemaDialect"] = "https://example.test/other" },
		func(root, op, config map[string]any) { op["$id"] = "https://example.test/schema" },
		func(root, op, config map[string]any) { config["$anchor"] = "layout" },
		func(root, op, config map[string]any) {
			config["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		},
		func(root, op, config map[string]any) {
			object(config["$defs"])["extra"] = map[string]any{"type": "string"}
		},
		func(root, op, config map[string]any) { config["description"] = strings.Repeat("x", 64<<10) },
		func(root, op, config map[string]any) {
			object(object(object(config["properties"])["rows"])["items"])["items"] = map[string]any{"$ref": "#/$defs/key", "type": "string"}
		},
		func(root, op, config map[string]any) {
			object(object(object(config["properties"])["rows"])["items"])["items"] = map[string]any{"$ref": "#/$defs/unknown"}
		},
		func(root, op, config map[string]any) {
			variants := object(object(config["$defs"])["keyVariant"])
			variants["$ref"] = "#/$defs/key"
		},
		func(root, op, config map[string]any) {
			paths := object(root["paths"])
			paths["/other"] = paths["/data/api/v1/resources/ignition/keyboard_layout"]
			delete(paths, "/data/api/v1/resources/ignition/keyboard_layout")
		},
	} {
		root := keyboardFixture(t, "POST")
		op := object(object(object(root["paths"])["/data/api/v1/resources/ignition/keyboard_layout"])["post"])
		schema := object(object(object(object(op["requestBody"])["content"])["application/json"])["schema"])
		config := object(object(object(schema["items"])["properties"])["config"])
		change(root, op, config)
		// Keep both sides equal so scope/reference guards are exercised, rather
		// than rejecting every case at the paired-schema equality check.
		b, _ := json.Marshal(config)
		var backup map[string]any
		_ = json.Unmarshal(b, &backup)
		object(object(schema["items"])["properties"])["backupConfig"] = backup
		before, _ := json.Marshal(root)
		if len(keyboardIdentityScopes(root)) != 0 {
			t.Fatal("unreviewed reference scope affected identity")
		}
		for _, adjustment := range normalizeIgnition(root) {
			if adjustment.Rule == "keyboard-local-definitions" {
				t.Fatal("unreviewed schema was expanded")
			}
		}
		after, _ := json.Marshal(root)
		if !bytes.Equal(before, after) {
			t.Fatal("failed qualification partially changed the model")
		}
	}
}

func TestKeyboardDefinitionsPreserveNewAssertionsAndRejectMismatchedPairs(t *testing.T) {
	t.Parallel()
	root := keyboardFixture(t, "POST")
	op := object(object(object(root["paths"])["/data/api/v1/resources/ignition/keyboard_layout"])["post"])
	schema := object(object(object(object(op["requestBody"])["content"])["application/json"])["schema"])
	properties := object(object(schema["items"])["properties"])
	for _, name := range []string{"config", "backupConfig"} {
		defs := object(object(properties[name])["$defs"])
		variants := object(defs["keyVariant"])["oneOf"].([]any)
		object(variants[0])["maxLength"] = 1
	}
	raw, _ := json.Marshal(root)
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, value := range []string{"A", "AB"} {
		body := `[{"name":"fixture","config":{"id":"layout-id","name":"TestLayout","title":"Test","alias":"EN","rows":[[{"lowercase":"` + value + `"}]]}}]`
		req, _ := http.NewRequest("POST", "http://gateway.test/data/api/v1/resources/ignition/keyboard_layout", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate(c.Operations()[0].Key, req)
		if err != nil || (len(issues) == 0) != (value == "A") {
			t.Fatalf("new assertion was lost: %v %v", issues, err)
		}
	}
	object(properties["config"])["description"] = "only the primary changed"
	before, _ := json.Marshal(root)
	if len(normalizeIgnition(root)) != 0 {
		t.Fatal("mismatched primary and backup schemas were expanded")
	}
	after, _ := json.Marshal(root)
	if !bytes.Equal(before, after) {
		t.Fatal("mismatched pair was partially changed")
	}
}
