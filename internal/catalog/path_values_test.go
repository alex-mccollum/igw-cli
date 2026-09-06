package catalog

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func pathValueCatalog(t *testing.T, version, template, schema, servers string) *Catalog {
	t.Helper()
	raw := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Path values","version":"test"},%s"paths":{%q:{"get":{"parameters":[{"in":"path","name":"value","required":true,"schema":%s}],"responses":{"200":{"description":"OK"}}}}}}`, version, servers, template, schema)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestExactPathValues(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, tt := range []struct {
			name, schema, value string
			valid               bool
		}{
			{"reserved and Unicode", `{"type":"string","enum":["a/b + ? # % = 日本"]}`, "a/b + ? # % = 日本", true},
			{"exact whitespace", `{"type":"string","enum":[" exact "]}`, " exact ", true},
			{"no whitespace coercion", `{"type":"string","enum":["exact"]}`, " exact ", false},
			{"enum plus constraint", `{"type":"string","enum":["a"],"minLength":2}`, "a", false},
			{"missing value", `{"type":"string"}`, "", false},
			{"integer beyond int64", `{"type":"integer","minimum":0}`, "18446744073709551616", true},
			{"integer minimum", `{"type":"integer","minimum":2}`, "1", false},
			{"integer maximum", `{"type":"integer","maximum":2}`, "3", false},
			{"integer spelling", `{"type":"integer"}`, "+1", false},
			{"exact minimum", `{"type":"number","minimum":9007199254740993}`, "9007199254740992", false},
			{"exact maximum", `{"type":"number","maximum":9007199254740993}`, "9007199254740993", true},
			{"decimal multiple", `{"type":"number","multipleOf":0.1}`, "0.3", true},
			{"decimal mismatch", `{"type":"number","multipleOf":0.1}`, "0.300000000000000000001", false},
			{"nonfinite", `{"type":"number"}`, "NaN", false},
			{"boolean enum", `{"type":"boolean","enum":[false]}`, "true", false},
			{"boolean valid", `{"type":"boolean","enum":[false]}`, "false", true},
			{"boolean alias", `{"type":"boolean"}`, "1", false},
			{"boolean case", `{"type":"boolean"}`, "TRUE", false},
			{"invalid UTF8", `{"type":"string"}`, "\xff", false},
			{"numeric limit", `{"type":"number"}`, "1e999999999999", false},
		} {
			t.Run(version+"/"+tt.name, func(t *testing.T) {
				c := pathValueCatalog(t, version, "/items/{value}", tt.schema, "")
				req, _ := http.NewRequest("GET", "http://gateway.test/items/"+url.PathEscape(tt.value), nil)
				beforePath, beforeRaw := req.URL.EscapedPath(), string(c.Raw())
				issues, err := c.Validate("GET /items/{value}", req)
				if err != nil || (len(issues) == 0) != tt.valid {
					t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
				}
				if req.URL.EscapedPath() != beforePath || string(c.Raw()) != beforeRaw {
					t.Fatal("validation rewrote wire input or vendor evidence")
				}
			})
		}
	}
}

func TestSelectedPathBinding(t *testing.T) {
	for _, tt := range []struct {
		name, template, path, servers, value string
		valid                                bool
	}{
		{"vendor prefix", "/prefix/items/{value}", "/prefix/items/known", `"servers":[{"url":"https://foreign.test/prefix"}],`, "known", true},
		{"literal mismatch", "/prefix/items/{value}", "/other/items/known", "", "known", false},
		{"too few segments", "/prefix/items/{value}", "/prefix", "", "known", false},
		{"too many segments", "/prefix/items/{value}", "/prefix/items/known/extra", "", "known", false},
		{"encoded slash", "/items/{value}", "/items/a%2fb", "", "a/b", true},
		{"decode once", "/items/{value}", "/items/a%252fb", "", "a%2fb", true},
		{"literal suffix", "/items/pre-{value}.json", "/items/pre-a%2Fb.json", "", "a/b", true},
		{"escaped literal", "/items/pre-{value}.json", "/items/%70re-known%2ejson", "", "known", true},
		{"literal regex", "/items/v1.0/{value}", "/items/v1x0/known", "", "known", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// A malformed request must produce a validation result, never panic.
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("path validation panicked: %v", recovered)
				}
			}()
			c := pathValueCatalog(t, "3.1.0", tt.template, fmt.Sprintf(`{"type":"string","enum":[%q]}`, tt.value), tt.servers)
			req, _ := http.NewRequest("GET", "http://gateway.test"+tt.path, nil)
			issues, err := c.Validate("GET "+tt.template, req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
			}
		})
	}
}

func TestPathValueInheritance(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE"} {
		for _, override := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/override=%t", method, override), func(t *testing.T) {
				local := ""
				if override {
					local = `"parameters":[{"in":"path","name":"value","required":true,"schema":{"type":"integer","minimum":0}}],`
				}
				raw := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Inherited paths","version":"test"},"paths":{"/items/{value}":{"parameters":[{"in":"path","name":"value","required":true,"schema":{"type":"integer","minimum":2}}],%q:{%s"responses":{"200":{"description":"OK"}}}}}}`, strings.ToLower(method), local)
				c, err := Parse([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				req, _ := http.NewRequest(method, "http://gateway.test/items/1", nil)
				issues, err := c.Validate(method+" /items/{value}", req)
				if err != nil || (len(issues) == 0) != override {
					t.Fatalf("override=%t: %+v %v", override, issues, err)
				}
			})
		}
	}
}

func TestPathBindingStructures(t *testing.T) {
	for _, tt := range []struct {
		template, path, parameters string
		valid                      bool
	}{
		{"/items/{left}.{right}", "/items/a.b", `[{"in":"path","name":"left","required":true,"schema":{"type":"string"}},{"in":"path","name":"right","required":true,"schema":{"type":"string"}}]`, true},
		{"/items/{left}.{right}", "/items/a.b.c", `[{"in":"path","name":"left","required":true,"schema":{"type":"string"}},{"in":"path","name":"right","required":true,"schema":{"type":"string"}}]`, false},
		{"/items/{value}/{value}", "/items/a/a", `[{"in":"path","name":"value","required":true,"schema":{"type":"string"}}]`, true},
		{"/items/{value}/{value}", "/items/a/b", `[{"in":"path","name":"value","required":true,"schema":{"type":"string"}}]`, false},
		{"/health", "/different", `[]`, false},
		{"/items/{value}", "/items/one,two", `[{"in":"path","name":"value","required":true,"schema":{"type":"array","items":{"type":"string"}}}]`, false},
		{"/items/{value}", "/items/.one", `[{"in":"path","name":"value","required":true,"style":"label","schema":{"type":"string"}}]`, false},
		{"/items/{value}", "/items/;value=one", `[{"in":"path","name":"value","required":true,"style":"matrix","schema":{"type":"string"}}]`, false},
		{"/items/{value}", "/items/%7B%7D", `[{"in":"path","name":"value","required":true,"content":{"application/json":{"schema":{"type":"object"}}}}]`, true},
	} {
		t.Run(tt.template+tt.path, func(t *testing.T) {
			raw := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Path bindings","version":"test"},"paths":{%q:{"get":{"parameters":%s,"responses":{"200":{"description":"OK"}}}}}}`, tt.template, tt.parameters)
			c, err := Parse([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			req, _ := http.NewRequest("GET", "http://gateway.test"+tt.path, nil)
			issues, err := c.Validate("GET "+tt.template, req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
			}
		})
	}
}
