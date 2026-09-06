package catalog

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSchema30ExactBodyConstraints(t *testing.T) {
	for _, tt := range []struct {
		name, schema, body string
		valid              bool
	}{
		{"exact minimum", `{"type":"number","minimum":9007199254740993}`, `9007199254740992`, false},
		{"exact maximum", `{"type":"number","maximum":9007199254740993}`, `9007199254740993`, true},
		{"exclusive minimum", `{"type":"number","minimum":9007199254740993,"exclusiveMinimum":true}`, `9007199254740993`, false},
		{"above exclusive minimum", `{"type":"number","minimum":9007199254740993,"exclusiveMinimum":true}`, `9007199254740994`, true},
		{"exclusive maximum", `{"type":"number","maximum":9007199254740993,"exclusiveMaximum":true}`, `9007199254740993`, false},
		{"below exclusive maximum", `{"type":"number","maximum":9007199254740993,"exclusiveMaximum":true}`, `9007199254740992`, true},
		{"inclusive maximum", `{"type":"number","maximum":9007199254740993,"exclusiveMaximum":false}`, `9007199254740993`, true},
		{"non-numeric exclusive bound", `{"type":"string","minimum":9007199254740993,"exclusiveMinimum":true}`, `"text"`, true},
		{"decimal maximum", `{"type":"number","maximum":0.300000000000000000001}`, `0.300000000000000000002`, false},
		{"nullable", `{"type":"string","nullable":true}`, `null`, true},
		{"nonnullable", `{"type":"string","nullable":false}`, `null`, false},
		{"nullable enum still applies", `{"type":"string","nullable":true,"enum":["a"]}`, `null`, false},
		{"nullable enum permits null", `{"type":"string","nullable":true,"enum":["a",null]}`, `null`, true},
		{"nullable requires local type", `{"nullable":true,"allOf":[{"type":"string"}]}`, `null`, false},
		{"nullable composition still applies", `{"type":"string","nullable":true,"allOf":[{"type":"string"}]}`, `null`, false},
		{"instance data is opaque", `{"type":"object","enum":[{"type":"string","nullable":true,"minimum":9007199254740993,"exclusiveMinimum":true}]}`, `{"type":"string","nullable":true,"minimum":9007199254740993,"exclusiveMinimum":true}`, true},
		{"nested schema constraints", `{"type":"object","properties":{"value":{"type":"number","minimum":9007199254740993}},"required":["value"]}`, `{"value":9007199254740992}`, false},
		{"discriminator present", `{"type":"object","discriminator":{"propertyName":"kind"}}`, `{"kind":"test"}`, true},
		{"discriminator missing", `{"type":"object","discriminator":{"propertyName":"kind"}}`, `{}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := bodyValueCatalog(t, "3.0.3", tt.schema, "application/json", true)
			before := string(c.Raw())
			req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			issues, err := c.Validate("POST /body", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
			}
			if string(c.Raw()) != before || !strings.Contains(before, tt.schema) || c.version != "3.0.3" {
				t.Fatal("private schema adaptation changed vendor evidence or version")
			}
		})
	}
}

func TestSchema30ExactQueryConstraints(t *testing.T) {
	for _, tt := range []struct {
		schema string
		values []string
		valid  bool
	}{
		{`{"type":"number","minimum":9007199254740993}`, []string{"9007199254740992"}, false},
		{`{"type":"number","maximum":9007199254740993}`, []string{"9007199254740993"}, true},
		{`{"type":"integer","nullable":true,"minimum":1}`, []string{"2"}, true},
		{`{"type":"integer","nullable":true,"minimum":1}`, []string{"null"}, false},
		{`{"type":"array","items":{"type":"integer","nullable":true,"minimum":1}}`, []string{"2", "3"}, true},
		{`{"type":"array","items":{"type":"integer","nullable":true,"minimum":1}}`, []string{"2", "0"}, false},
	} {
		raw := fmt.Sprintf(`{"openapi":"3.0.3","info":{"title":"Exact query","version":"test"},"paths":{"/values":{"get":{"parameters":[{"in":"query","name":"value","required":true,"schema":%s}],"responses":{"200":{"description":"OK"}}}}}}`, tt.schema)
		c, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer c.Close()
			req, _ := http.NewRequest("GET", "http://gateway.test/values?"+url.Values{"value": tt.values}.Encode(), nil)
			issues, err := c.Validate("GET /values", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
			}
		}()
	}
}

func TestSchema30RecursiveReferenceConstraints(t *testing.T) {
	const raw = `{"openapi":"3.0.3","info":{"title":"Recursive exact schema","version":"test"},"paths":{"/body":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Node"}}}},"responses":{"200":{"description":"OK"}}}}},"components":{"schemas":{"Node":{"type":"object","nullable":true,"properties":{"value":{"type":"number","minimum":9007199254740993},"next":{"$ref":"#/components/schemas/Node"}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tt := range []struct {
		body  string
		valid bool
	}{
		{`{"value":9007199254740993,"next":null}`, true},
		{`{"next":{"value":9007199254740993}}`, true},
		{`{"next":{"value":9007199254740992}}`, false},
	} {
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("POST /body", req)
		if err != nil || (len(issues) == 0) != tt.valid {
			t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
		}
	}
}
