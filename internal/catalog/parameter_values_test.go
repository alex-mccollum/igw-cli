package catalog

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNamedQueryParameterContract(t *testing.T) {
	for _, tt := range []struct {
		name, schema string
		values       []string
		valid        bool
	}{
		{"exact reserved string", `{"type":"string","enum":["text + & % # / = 日本"]}`, []string{"text + & % # / = 日本"}, true},
		{"exact whitespace string", `{"type":"string","enum":[" text "]}`, []string{" text "}, true},
		{"empty string allowed by schema", `{"type":"string","maxLength":0}`, []string{""}, true},
		{"empty string violates schema", `{"type":"string","minLength":1}`, []string{""}, false},
		{"one scalar value", `{"type":"integer","minimum":1}`, []string{"2"}, true},
		{"repeated scalar is ambiguous", `{"type":"integer","minimum":1}`, []string{"2", "3"}, false},
		{"integer beyond int64", `{"type":"integer","minimum":0}`, []string{"18446744073709551616"}, true},
		{"fractional integer", `{"type":"integer"}`, []string{"1.5"}, false},
		{"precise numeric constant", `{"type":"number","const":9007199254740993}`, []string{"9007199254740993"}, true},
		{"distinct adjacent constant", `{"type":"number","const":9007199254740993}`, []string{"9007199254740992"}, false},
		{"precise schema minimum", `{"type":"number","minimum":9007199254740993}`, []string{"9007199254740992"}, false},
		{"precise schema maximum", `{"type":"number","maximum":9007199254740993}`, []string{"9007199254740993"}, true},
		{"precise decimal maximum", `{"type":"number","maximum":0.300000000000000000001}`, []string{"0.300000000000000000002"}, false},
		{"decimal multiple", `{"type":"number","multipleOf":0.1}`, []string{"0.3"}, true},
		{"precise decimal mismatch", `{"type":"number","multipleOf":0.1}`, []string{"0.300000000000000000001"}, false},
		{"boolean enum allowed", `{"type":"boolean","enum":[false]}`, []string{"false"}, true},
		{"boolean enum rejected", `{"type":"boolean","enum":[false]}`, []string{"true"}, false},
		{"boolean const rejected", `{"type":"boolean","const":false}`, []string{"true"}, false},
		{"boolean numeric alias", `{"type":"boolean"}`, []string{"1"}, false},
		{"boolean capital alias", `{"type":"boolean"}`, []string{"TRUE"}, false},
		{"whole array size", `{"type":"array","minItems":2,"maxItems":3,"items":{"type":"string"}}`, []string{"one", "two"}, true},
		{"whole array maximum", `{"type":"array","maxItems":2,"items":{"type":"string"}}`, []string{"one", "two", "three"}, false},
		{"whole array uniqueness", `{"type":"array","uniqueItems":true,"items":{"type":"string"}}`, []string{"one", "one"}, false},
		{"array item length", `{"type":"array","items":{"type":"string","minLength":2}}`, []string{"a"}, false},
		{"array item pattern", `{"type":"array","items":{"type":"string","pattern":"^[a-z]+$"}}`, []string{"abc", "123"}, false},
		{"comma belongs to one exploded item", `{"type":"array","minItems":1,"maxItems":1,"items":{"type":"string","const":"red,blue"}}`, []string{"red,blue"}, true},
		{"array number constraint", `{"type":"array","items":{"type":"number","multipleOf":0.25}}`, []string{"0.3"}, false},
		{"array boolean constraint", `{"type":"array","items":{"type":"boolean","enum":[false]}}`, []string{"false", "true"}, false},
		{"array precise integer", `{"type":"array","items":{"type":"integer","minimum":0}}`, []string{"18446744073709551616"}, true},
		{"numeric exponent limit", `{"type":"number","minimum":0}`, []string{"1e999999999999999999999"}, false},
		{"numeric text limit", `{"type":"integer","minimum":0}`, []string{strings.Repeat("1", maxParameterNumberChars+1)}, false},
		{"nonfinite number", `{"type":"number"}`, []string{"NaN"}, false},
		{"invalid UTF8", `{"type":"string"}`, []string{"\xff"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var schema any
			d := json.NewDecoder(strings.NewReader(tt.schema))
			d.UseNumber()
			if err := d.Decode(&schema); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(map[string]any{
				"openapi": "3.1.0", "info": map[string]string{"title": "Synthetic parameter contract", "version": "test"},
				"paths": map[string]any{"/params": map[string]any{"get": map[string]any{
					"parameters": []any{map[string]any{"name": "value", "in": "query", "style": "form", "explode": true, "required": true, "schema": schema}},
					"responses":  map[string]any{"200": map[string]string{"description": "OK"}},
				}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			c, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			query := url.Values{"value": tt.values}.Encode()
			req, _ := http.NewRequest("GET", "http://gateway.test/params?"+query, nil)
			issues, err := c.Validate("GET /params", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
			}
			if req.URL.RawQuery != query || string(c.Raw()) != string(raw) {
				t.Fatal("parameter validation changed wire input or vendor bytes")
			}
		})
	}
}

func TestNamedQueryInheritanceAndFilterOwnership(t *testing.T) {
	for _, override := range []bool{false, true} {
		c := changedFilterCatalog(t, func(item map[string]any) {
			inherited := map[string]any{"name": "ids", "in": "query", "required": true, "schema": map[string]any{"type": "array", "minItems": 2, "items": map[string]any{"type": "string"}}}
			item["parameters"] = []any{inherited}
			if override {
				op := item["get"].(map[string]any)
				op["parameters"] = append(op["parameters"].([]any), map[string]any{"name": "ids", "in": "query", "schema": inherited["schema"]})
			}
		})
		for _, tt := range []struct {
			query string
			valid bool
		}{
			{"name=present&ids=one&ids=two", true},
			{"name=present&ids=one&ids=two&name%5Beq%5D=filter", true},
			{"name=present&ids=one&name%5Beq%5D=filter", false},
			{"name=present", override},
			{"name=present&name%5Beq%5D=filter", override},
			{"ids=one&ids=two&name%5Beq%5D=filter", false},
		} {
			req, _ := http.NewRequest("GET", "http://gateway.test/items?"+tt.query, nil)
			issues, err := c.Validate("GET /items", req)
			if err != nil || (len(issues) == 0) != tt.valid || req.URL.RawQuery != tt.query {
				t.Fatalf("override=%t valid=%t: %+v %v", override, tt.valid, issues, err)
			}
		}
	}
}

func TestNamedQueryOpenAPI30(t *testing.T) {
	const spec = `{"openapi":"3.0.3","info":{"title":"Synthetic 3.0 query","version":"test"},"paths":{"/values":{"get":{"parameters":[{"in":"query","name":"enabled","required":true,"schema":{"type":"boolean","enum":[false]}},{"in":"query","name":"values","required":true,"schema":{"type":"array","minItems":2,"uniqueItems":true,"items":{"type":"integer","minimum":0}}}],"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tt := range []struct {
		query string
		valid bool
	}{
		{"enabled=false&values=1&values=2", true},
		{"enabled=false&values=1&values=18446744073709551616", true},
		{"enabled=true&values=1&values=2", false},
		{"enabled=false&values=1&values=1", false},
		{"enabled=false&values=1&values=-1", false},
		{"enabled=false&values=1", false},
	} {
		req, _ := http.NewRequest("GET", "http://gateway.test/values?"+tt.query, nil)
		issues, err := c.Validate("GET /values", req)
		if err != nil || (len(issues) == 0) != tt.valid {
			t.Fatalf("3.0 valid=%t: %+v %v", tt.valid, issues, err)
		}
	}
}
