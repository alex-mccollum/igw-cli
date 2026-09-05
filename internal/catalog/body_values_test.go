package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestJSONBodyBooleanReferencesPreserveInspection(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		raw := []byte(fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Synthetic Boolean reference","version":"test"},"paths":{"/body":{"post":{"requestBody":{"$ref":"#/components/requestBodies/Input"},"responses":{"200":{"description":"OK"}}}}},"components":{"schemas":{"Value":%t},"requestBodies":{"Input":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Value"}}}}}}}`, allowed))
		c, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(`null`))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("POST /body", req)
		if err != nil || (len(issues) == 0) != allowed {
			t.Fatalf("referenced Boolean schema allowed=%t: %+v %v", allowed, issues, err)
		}
		description, err := c.Describe("POST /body")
		if err != nil || !bytes.Equal(c.Raw(), raw) || !bytes.Contains(description.Components, []byte(fmt.Sprintf(`"Value":%t`, allowed))) {
			t.Fatal("parser representation replaced original schema inspection")
		}
		if old, err := Parse(bytes.Replace(raw, []byte(`"3.1.0"`), []byte(`"3.0.3"`), 1)); err == nil {
			old.Close()
			t.Fatal("Boolean schema adaptation bypassed OpenAPI 3.0 document validation")
		}
	}
}

func bodyValueCatalog(t *testing.T, version, schema, contentType string, required bool) *Catalog {
	t.Helper()
	raw := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Synthetic exact JSON body","version":"test"},"paths":{"/body":{"post":{"requestBody":{"required":%t,"content":{%q:{"schema":%s}}},"responses":{"200":{"description":"OK"}}}}}}`, version, required, contentType, schema)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestJSONBodyExactValues(t *testing.T) {
	for _, tt := range []struct {
		name, schema, body string
		valid              bool
	}{
		{"exact integer", `{"type":"integer","const":9007199254740993}`, `9007199254740993`, true},
		{"adjacent integer", `{"type":"integer","const":9007199254740993}`, `9007199254740992`, false},
		{"precise minimum", `{"type":"number","minimum":9007199254740993}`, `9007199254740992`, false},
		{"precise maximum", `{"type":"number","maximum":9007199254740993}`, `9007199254740993`, true},
		{"precise decimal", `{"type":"number","maximum":0.300000000000000000001}`, `0.300000000000000000002`, false},
		{"decimal multiple", `{"type":"number","multipleOf":0.1}`, `0.3`, true},
		{"decimal mismatch", `{"type":"number","multipleOf":0.1}`, `0.300000000000000000001`, false},
		{"large finite number", `{"type":"number","minimum":0}`, `1e400`, true},
		{"fractional integer", `{"type":"integer"}`, `1.000000000000000000001`, false},
		{"integral decimal spelling", `{"type":"integer"}`, `1.0`, true},
		{"integral exponent spelling", `{"type":"integer"}`, `10e-1`, true},
		{"array distinct numbers", `{"type":"array","uniqueItems":true,"items":{"type":"integer"}}`, `[9007199254740992,9007199254740993]`, true},
		{"array equivalent spellings", `{"type":"array","uniqueItems":true,"items":{"type":"number"}}`, `[1,1.0,10e-1]`, false},
		{"nested exact number", `{"type":"object","required":["value"],"properties":{"value":{"type":"integer","const":9007199254740993}}}`, `{"value":9007199254740993}`, true},
		{"explicit null", `{"type":["object","null"]}`, `null`, true},
		{"boolean true schema", `true`, `null`, true},
		{"boolean false schema", `false`, `null`, false},
		{"false property schema", `{"type":"object","properties":{"value":false}}`, `{"value":1}`, false},
		{"not false schema", `{"not":false}`, `null`, true},
		{"allOf false schema", `{"allOf":[false]}`, `{}`, false},
		{"anyOf false schema", `{"anyOf":[false,{"type":"integer"}]}`, `"text"`, false},
		{"tuple false schema", `{"type":"array","prefixItems":[false]}`, `[1]`, false},
		{"contains false schema", `{"type":"array","contains":false}`, `[1]`, false},
		{"conditional false schema", `{"if":true,"then":false}`, `{}`, false},
		{"pattern false schema", `{"type":"object","patternProperties":{"^x-":false}}`, `{"x-value":1}`, false},
		{"data booleans remain data", `{"const":{"schema":false,"properties":{"value":false}}}`, `{"schema":false,"properties":{"value":false}}`, true},
		{"null is not object", `{"type":"object"}`, `null`, false},
		{"null is not omitted", `{"type":"object","required":["value"],"properties":{"value":{"type":"null"}}}`, `{}`, false},
		{"present null property", `{"type":"object","required":["value"],"properties":{"value":{"type":"null"}}}`, `{"value":null}`, true},
		{"false is not omitted", `{"type":"boolean","const":false}`, `false`, true},
		{"empty string", `{"type":"string","maxLength":0}`, `""`, true},
		{"exact text", `{"type":"string","const":" text + & % # / = 日本 "}`, `" text + & % # / = 日本 "`, true},
		{"duplicate object key", `{"type":"object"}`, `{"value":1,"value":2}`, false},
		{"escaped duplicate key", `{"type":"object"}`, `{"value":1,"\u0076alue":2}`, false},
		{"nested duplicate key", `{"type":"array"}`, `[{"value":1,"value":2}]`, false},
		{"second JSON value", `{"type":"object"}`, `{} {}`, false},
		{"whitespace is not null", `{"type":"null"}`, " \n\t ", false},
		{"invalid UTF8", `{"type":"string"}`, "\"\xff\"", false},
		{"paired Unicode escapes", `{"type":"string","const":"😀"}`, `"\ud83d\ude00"`, true},
		{"high surrogate alone", `{"type":"string"}`, `"\ud83d"`, false},
		{"low surrogate alone", `{"type":"string"}`, `"\ude00"`, false},
		{"reversed surrogates", `{"type":"string"}`, `"\ude00\ud83d"`, false},
		{"surrogate before ASCII", `{"type":"string"}`, `"\ud83d\u0041"`, false},
		{"literal escaped backslash", `{"type":"string"}`, `"\\ud83d"`, true},
		{"explicit replacement character", `{"type":"string","const":"�"}`, `"\ufffd"`, true},
		{"numeric text bound", `{"type":"number"}`, strings.Repeat("1", maxParameterNumberChars+1), false},
		{"numeric exponent bound", `{"type":"object"}`, `{"value":1e999999999999999999999}`, false},
		{"negative exponent bound", `{"type":"number"}`, `1e-4097`, false},
		{"numeric exponent edge", `{"type":"number","minimum":0}`, `1e4096`, true},
		{"excessive nesting", `{}`, strings.Repeat("[", 257) + `0` + strings.Repeat("]", 257), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := bodyValueCatalog(t, "3.1.0", tt.schema, "application/json", true)
			req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			issues, err := c.Validate("POST /body", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
			}
		})
	}
}

func TestJSONBodyMediaSelection(t *testing.T) {
	for _, tt := range []struct {
		name, content, contentType, body string
		valid                            bool
	}{
		{"case and parameters", `"application/json":{"schema":{"type":"integer","const":9007199254740993}}`, "Application/JSON; charset=utf-8", `9007199254740993`, true},
		{"suffix", `"application/example+json":{"schema":{"type":"integer","const":9007199254740993}}`, "application/example+json", `9007199254740993`, true},
		{"suffix is not declaration", `"application/json":{"schema":{"type":"integer"}}`, "application/example+json", `1`, false},
		{"exact precedes wildcard", `"*/*":{"schema":{"type":"string"}},"application/*":{"schema":{"type":"boolean"}},"application/json":{"schema":{"type":"integer"}}`, "application/json", `1`, true},
		{"specific range precedes wildcard", `"*/*":{"schema":{"type":"string"}},"application/*":{"schema":{"type":"integer"}}`, "application/example+json", `1`, true},
		{"wildcard preserves numbers", `"*/*":{"schema":{"type":"integer","const":9007199254740993}}`, "application/json", `9007199254740993`, true},
		{"ambiguous declaration", `"application/json":{"schema":{"type":"integer"}},"Application/JSON":{"schema":{"type":"string"}}`, "application/json", `1`, false},
		{"invalid media type", `"application/json":{"schema":{"type":"object"}}`, "application/json; broken=", `{}`, false},
		{"malformed media type", `"application/json":{"schema":{"type":"object"}}`, "json", `{}`, false},
		{"no declared type", `"application/xml":{"schema":{"type":"object"}}`, "application/json", `{}`, false},
		{"opaque presence", `"application/json":{}`, "application/json", `opaque contract`, true},
		{"opaque required absence", `"application/json":{}`, "application/json", ``, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Synthetic JSON media selection","version":"test"},"paths":{"/body":{"post":{"requestBody":{"required":true,"content":{%s}},"responses":{"200":{"description":"OK"}}}}}}`, tt.content)
			c, err := Parse([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req, _ := http.NewRequest("POST", "http://gateway.test/body", body)
			req.Header.Set("Content-Type", tt.contentType)
			issues, err := c.Validate("POST /body", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
			}
		})
	}
}

type failingBodyReader struct{}

func (failingBodyReader) Read([]byte) (int, error) { return 0, errors.New("private-read-error") }

func TestJSONBodyReadAndCompilationFailures(t *testing.T) {
	c := bodyValueCatalog(t, "3.1.0", `{"type":"object"}`, "application/json", true)
	for _, tt := range []struct {
		name, rule string
		body       io.Reader
	}{
		{"size", "body_limit", strings.NewReader(strings.Repeat(" ", MaxJSONBodyBytes+1))},
		{"read", "body_read", failingBodyReader{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "http://gateway.test/body", tt.body)
			req.Header.Set("Content-Type", "application/json")
			issues, err := c.Validate("POST /body", req)
			if err != nil || len(issues) != 1 || issues[0].Rule != tt.rule {
				t.Fatalf("expected %s: %+v %v", tt.rule, issues, err)
			}
		})
	}
	broken := bodyValueCatalog(t, "3.1.0", `{"type":"string","pattern":"["}`, "application/json", true)
	req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(`"private-body-text"`))
	req.Header.Set("Content-Type", "application/json")
	issues, err := broken.Validate("POST /body", req)
	if !errors.Is(err, ErrSchemaCompilation) || len(issues) != 0 {
		t.Fatalf("schema compilation error lost: %+v %v", issues, err)
	}
}

func TestJSONBodyPresenceAndDirection(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, required := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/required=%t", version, required), func(t *testing.T) {
				c := bodyValueCatalog(t, version, `{"type":"object","required":["generated","secret"],"properties":{"generated":{"type":"integer","readOnly":true},"secret":{"type":"string","writeOnly":true}}}`, "application/json", required)
				for _, tt := range []struct {
					body  string
					valid bool
				}{
					{"", !required}, {`{"secret":"private-secret"}`, true}, {`{}`, false},
					{`{"generated":1}`, false}, {`{"secret":false}`, false},
				} {
					var body io.Reader
					if tt.body != "" {
						body = strings.NewReader(tt.body)
					}
					req, _ := http.NewRequest("POST", "http://gateway.test/body", body)
					req.Header.Set("Content-Type", "application/json")
					before := string(c.Raw())
					issues, err := c.Validate("POST /body", req)
					if err != nil || (len(issues) == 0) != tt.valid {
						t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
					}
					encoded, _ := json.Marshal(issues)
					if strings.Contains(string(encoded), "private-secret") || string(c.Raw()) != before {
						t.Fatal("body validation exposed values or changed vendor bytes")
					}
				}
			})
		}
	}
	for _, tt := range []struct {
		schema string
		valid  bool
	}{{`{"type":"object","nullable":true}`, true}, {`{"type":"object"}`, false}} {
		c := bodyValueCatalog(t, "3.0.3", tt.schema, "application/json", true)
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(`null`))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("POST /body", req)
		if err != nil || (len(issues) == 0) != tt.valid {
			t.Fatalf("3.0 nullable valid=%t: %+v %v", tt.valid, issues, err)
		}
	}
}
