package catalog

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func contentParameterRequest(t *testing.T, version, location, media, schema, text string) (*Catalog, *http.Request, string) {
	t.Helper()
	path := "/values"
	if location == "path" {
		path += "/{value}"
	}
	raw := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Synthetic content parameter","version":"test"},"paths":{%q:{"get":{"parameters":[{"name":"value","in":%q,"required":true,"content":{%q:{"schema":%s}}}],"responses":{"200":{"description":"OK"}}}}}}`, version, path, location, media, schema)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	endpoint := "http://gateway.test/values"
	if location == "query" {
		endpoint += "?" + url.Values{"value": {text}}.Encode()
	} else {
		endpoint += "/" + url.PathEscape(text)
	}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, req, "GET " + path
}

func TestContentQueryAndPathValues(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, location := range []string{"query", "path"} {
			for _, tc := range []struct {
				name, media, schema, value string
				valid                      bool
			}{
				{"nested precise integer", "application/json", `{"type":"object","required":["id"],"properties":{"id":{"type":"integer","minimum":9007199254740993}},"additionalProperties":false}`, `{"id":9007199254740993}`, true},
				{"adjacent integer refused", "application/json", `{"type":"object","required":["id"],"properties":{"id":{"type":"integer","minimum":9007199254740993}}}`, `{"id":9007199254740992}`, false},
				{"complete array", "application/example+json", `{"type":"array","minItems":2,"items":{"type":"string"}}`, `["", "a,b/+%2F?&= 日本"]`, true},
				{"array constraint", "application/json", `{"type":"array","uniqueItems":true,"items":{"type":"string"}}`, `["same","same"]`, false},
				{"empty array", "application/json", `{"type":"array","maxItems":0}`, `[]`, true},
				{"empty JSON string", "application/json", `{"type":"string","maxLength":0}`, `""`, true},
				{"integral decimal", "application/json", `{"type":"integer","minimum":1}`, `1.0`, true},
				{"boolean", "application/json", `{"type":"boolean","enum":[false]}`, `false`, true},
				{"duplicate JSON key", "application/json", `{"type":"object"}`, `{"id":1,"id":2}`, false},
				{"trailing JSON", "application/json", `{"type":"object"}`, `{} {}`, false},
				{"malformed Unicode", "application/json", `{"type":"string"}`, `"\ud800"`, false},
				{"numeric work limit", "application/json", `{"type":"number"}`, `1e4097`, false},
				{"literal text", "text/plain; charset=utf-8", `{"type":"string","enum":[" text/+%2F?&= 日本 "]}`, " text/+%2F?&= 日本 ", true},
				{"text constraint", "text/plain", `{"type":"string","minLength":2}`, "a", false},
				{"ASCII charset", "text/plain; charset=us-ascii", `{"type":"string"}`, "日本", false},
				{"unsupported media", "application/xml", `{"type":"object"}`, "<value/>", false},
			} {
				t.Run(version+"/"+location+"/"+tc.name, func(t *testing.T) {
					c, req, key := contentParameterRequest(t, version, location, tc.media, tc.schema, tc.value)
					before, raw := req.URL.String(), string(c.Raw())
					issues, err := c.Validate(key, req)
					if err != nil || (len(issues) == 0) != tc.valid {
						t.Fatalf("valid=%t: %v %v", tc.valid, issues, err)
					}
					if req.URL.String() != before || string(c.Raw()) != raw {
						t.Fatal("content validation changed the wire input or original document")
					}
				})
			}
		}
	}
}

func TestContentQueryAndPathPresence(t *testing.T) {
	for _, location := range []string{"query", "path"} {
		for _, version := range []string{"3.0.3", "3.1.0"} {
			schema := `{"type":"object","nullable":true}`
			if version == "3.1.0" {
				schema = `{"type":["null","object"]}`
			}
			c, req, key := contentParameterRequest(t, version, location, "application/json", schema, "null")
			if issues, err := c.Validate(key, req); err != nil || len(issues) != 0 {
				t.Fatalf("explicit nullable JSON failed: %v %v", issues, err)
			}
			if location == "query" {
				req.URL.RawQuery += "&value=null"
				if issues, err := c.Validate(key, req); err != nil || len(issues) == 0 {
					t.Fatalf("repeated content parameter was accepted: %v %v", issues, err)
				}
				req.URL.RawQuery = ""
			} else {
				req.URL.Path, req.URL.RawPath = "/values/", ""
			}
			if issues, err := c.Validate(key, req); err != nil || len(issues) == 0 {
				t.Fatalf("missing required parameter was accepted: %v %v", issues, err)
			}
			textCatalog, textReq, textKey := contentParameterRequest(t, version, location, "text/plain", `{"type":"string","maxLength":0}`, "")
			if issues, err := textCatalog.Validate(textKey, textReq); err != nil || (len(issues) == 0) != (location == "query") {
				t.Fatalf("empty text query/path presence changed: %v %v", issues, err)
			}
		}
	}
}

func TestContentQueryOptionalAndFilterOwnership(t *testing.T) {
	c := changedFilterCatalog(t, func(item map[string]any) {
		op := item["get"].(map[string]any)
		op["parameters"] = append(op["parameters"].([]any), map[string]any{
			"in": "query", "name": "options", "content": map[string]any{"application/json": map[string]any{
				"schema": map[string]any{"type": "object", "required": []string{"enabled"}, "properties": map[string]any{"enabled": map[string]any{"type": "boolean"}}},
			}},
		})
	})
	for _, tc := range []struct {
		query string
		valid bool
	}{
		{"name=present&name%5Beq%5D=filter", true},
		{"name=present&name%5Beq%5D=filter&options=" + url.QueryEscape(`{"enabled":false}`), true},
		{"name=present&name%5Beq%5D=filter&options=" + url.QueryEscape(`{"enabled":"false"}`), false},
		{"name=present&options=&options=%7B%7D", false},
	} {
		req, _ := http.NewRequest("GET", "http://gateway.test/items?"+tc.query, nil)
		issues, err := c.Validate("GET /items", req)
		if err != nil || (len(issues) == 0) != tc.valid {
			t.Fatalf("filter peer valid=%t: %v %v", tc.valid, issues, err)
		}
	}
	// Optional content needs no decoder when absent, even if its media is not
	// supported. A present value still fails instead of bypassing validation.
	for _, supplied := range []bool{false, true} {
		raw := strings.ReplaceAll(string(c.Raw()), "application/json", "application/xml")
		unsupported, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer unsupported.Close()
		query := "name=present"
		if supplied {
			query += "&options=value"
		}
		req, _ := http.NewRequest("GET", "http://gateway.test/items?"+query, nil)
		issues, err := unsupported.Validate("GET /items", req)
		if err != nil || (len(issues) == 0) == supplied {
			t.Fatalf("optional unsupported content supplied=%t: %v %v", supplied, issues, err)
		}
	}
}

func TestContentParameterReferencesAndBounds(t *testing.T) {
	for _, location := range []string{"query", "path"} {
		c, _, key := contentParameterRequest(t, "3.1.0", location, "application/json", `{"type":"object"}`, "{}")
		raw := strings.Replace(string(c.Raw()), `"schema":{"type":"object"}`, `"schema":{"$ref":"#/components/schemas/Node"}`, 1)
		raw = strings.TrimSuffix(raw, "}") + `,"components":{"schemas":{"Node":{"type":"object","required":["id"],"properties":{"id":{"type":"integer","minimum":9007199254740993},"child":{"$ref":"#/components/schemas/Node"}}}}}}`
		referenced, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer referenced.Close()
		for _, tc := range []struct {
			value string
			valid bool
		}{
			{`{"id":9007199254740993,"child":{"id":9007199254740993}}`, true},
			{`{"id":9007199254740993,"child":{"id":9007199254740992}}`, false},
			{"null", false},
		} {
			_, req, _ := contentParameterRequest(t, "3.1.0", location, "application/json", `{"type":"object"}`, tc.value)
			issues, err := referenced.Validate(key, req)
			if err != nil || (len(issues) == 0) != tc.valid {
				t.Fatalf("referenced content valid=%t: %v %v", tc.valid, issues, err)
			}
		}
	}
	// The shared decoder's work bound is checked before allocating JSON views.
	c, _, _ := contentParameterRequest(t, "3.1.0", "query", "application/json", `{"type":"string"}`, `""`)
	op, _ := c.Resolve("GET /values")
	view, err := c.requestContract(op)
	if err != nil {
		t.Fatal(err)
	}
	p := view.Operation.Parameters[0]
	if _, _, rule := parameterContentValue(p, strings.Repeat("x", MaxJSONBodyBytes+1)); rule != "parameter_limit" {
		t.Fatalf("oversized parameter entered decoding: %s", rule)
	}
}

func TestContentParameterOverridesInheritedSchema(t *testing.T) {
	for _, location := range []string{"query", "path"} {
		c, req, key := contentParameterRequest(t, "3.1.0", location, "application/json", `{"type":"object"}`, "{}")
		inherited := fmt.Sprintf(`"parameters":[{"in":%q,"name":"value","required":true,"schema":{"type":"boolean"}}],"get":`, location)
		raw := strings.Replace(string(c.Raw()), `"get":`, inherited, 1)
		overridden, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer overridden.Close()
		if issues, err := overridden.Validate(key, req); err != nil || len(issues) != 0 {
			t.Fatalf("operation content did not override inherited schema: %v %v", issues, err)
		}
	}
}
