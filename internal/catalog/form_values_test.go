package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestURLEncodedBodyContract(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, tc := range []struct {
			name, schema, encoding, body string
			valid, unsupported           bool
		}{
			{"literal string", `{"type":"string","enum":[" +&=%2F 日本 "]}`, `{}`, "value=+%2B%26%3D%252F+%E6%97%A5%E6%9C%AC+", true, false},
			{"empty string", `{"type":"string","maxLength":0}`, `{}`, "value=", true, false},
			{"required property", `{"type":"string"}`, `{}`, "", false, false},
			{"duplicate scalar", `{"type":"string"}`, `{}`, "value=a&value=b", false, false},
			{"exact integer", `{"type":"integer","minimum":9007199254740993}`, `{}`, "value=9007199254740993", true, false},
			{"adjacent integer", `{"type":"integer","minimum":9007199254740993}`, `{}`, "value=9007199254740992", false, false},
			{"precise fraction", `{"type":"number","multipleOf":0.1}`, `{}`, "value=0.3", true, false},
			{"fraction mismatch", `{"type":"number","multipleOf":0.1}`, `{}`, "value=0.300000000000000000001", false, false},
			{"boolean", `{"type":"boolean","enum":[false]}`, `{}`, "value=false", true, false},
			{"boolean spelling", `{"type":"boolean"}`, `{}`, "value=0", false, false},
			{"default object JSON", `{"type":"object","required":["id"],"properties":{"id":{"type":"integer","minimum":9007199254740993}},"additionalProperties":false}`, `{}`, `value=%7B%22id%22%3A9007199254740993%7D`, true, false},
			{"JSON constraint", `{"type":"object","required":["id"]}`, `{}`, `value=%7B%7D`, false, false},
			{"JSON duplicate", `{"type":"object"}`, `{}`, `value=%7B%22id%22:1,%22id%22:2%7D`, false, false},
			{"JSON trailing", `{"type":"object"}`, `{}`, `value=%7B%7D%7B%7D`, false, false},
			{"explicit JSON string", `{"type":"string","enum":["null"]}`, `{"value":{"contentType":"application/json"}}`, `value=%22null%22`, true, false},
			{"explicit JSON array", `{"type":"array","items":{"type":"integer"},"maxItems":0}`, `{"value":{"contentType":"application/json"}}`, `value=%5B%5D`, true, false},
			{"exploded array", `{"type":"array","items":{"type":"string"},"minItems":2,"uniqueItems":true}`, `{"value":{"style":"form"}}`, `value=a%2Cb&value=`, true, false},
			{"array cardinality", `{"type":"array","items":{"type":"string"},"minItems":2}`, `{"value":{"explode":true}}`, `value=a%2Cb`, false, false},
			{"whole array uniqueness", `{"type":"array","items":{"type":"number"},"uniqueItems":true}`, `{"value":{"explode":true}}`, `value=1&value=1.0`, false, false},
			{"array numeric constraint", `{"type":"array","items":{"type":"integer","minimum":2}}`, `{"value":{"style":"form"}}`, `value=2&value=1`, false, false},
			{"explicit false selects style", `{"type":"array","items":{"type":"string"},"minItems":2}`, `{"value":{"allowReserved":false,"contentType":"application/xml"}}`, `value=a&value=b`, true, false},
			{"style ignores content type", `{"type":"integer"}`, `{"value":{"style":"form","contentType":"application/json"}}`, `value=2`, true, false},
			{"XML", `{"type":"string"}`, `{"value":{"contentType":"application/xml"}}`, `value=text`, false, true},
			{"text media charset", `{"type":"integer","minimum":2}`, `{"value":{"contentType":"text/plain; charset=utf-8"}}`, `value=2`, true, false},
			{"byte encoding", `{"type":"string","format":"byte"}`, `{}`, `value=YQ%3D%3D`, false, true},
			{"implicit array", `{"type":"array","items":{"type":"string"}}`, `{}`, `value=a&value=b`, false, false},
			{"packed array", `{"type":"array","items":{"type":"string"}}`, `{"value":{"explode":false}}`, `value=a,b`, false, true},
			{"exploded object", `{"type":"object"}`, `{"value":{"style":"form"}}`, `value=a,b`, false, true},
			{"deep object", `{"type":"object"}`, `{"value":{"style":"deepObject","explode":true}}`, `value=%7B%7D`, false, true},
			{"reserved expansion", `{"type":"string"}`, `{"value":{"allowReserved":true}}`, `value=a`, false, true},
			{"unknown field", `{"type":"string"}`, `{}`, `value=a&unknown=b`, false, true},
			{"invalid percent", `{"type":"string"}`, `{}`, `value=%ZZ`, false, false},
			{"invalid Unicode", `{"type":"string"}`, `{}`, `value=%FF`, false, false},
			{"semicolon", `{"type":"string"}`, `{}`, `value=a;b`, false, false},
			{"numeric work limit", `{"type":"number"}`, `{}`, `value=1e4097`, false, false},
		} {
			t.Run(version+"/"+tc.name, func(t *testing.T) {
				raw := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Synthetic form contract","version":"test"},"paths":{"/body":{"post":{"requestBody":{"$ref":"#/components/requestBodies/Input"},"responses":{"200":{"description":"OK"}}}}},"components":{"schemas":{"Field":%s},"requestBodies":{"Input":{"required":true,"content":{"application/x-www-form-urlencoded":{"schema":{"type":"object","required":["value"],"properties":{"value":{"$ref":"#/components/schemas/Field"}},"additionalProperties":false},"encoding":%s}}}}}}`, version, tc.schema, tc.encoding)
				c, err := Parse([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				got, err := c.ValidateRequest("POST /body", req)
				if tc.unsupported {
					if !errors.Is(err, ErrUnsupportedBodyEncoding) {
						t.Fatalf("unsupported declaration: %+v %v", got, err)
					}
				} else if err != nil || (len(got.Issues) == 0) != tc.valid {
					t.Fatalf("valid=%t: %+v %v", tc.valid, got, err)
				}
				if tc.valid && got.Coverage != ValidationSchema {
					t.Fatalf("lost schema coverage: %+v", got)
				}
			})
		}
	}
}

func TestURLEncodedWholeObjectAssertions(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"value":{"type":"string"}},"not":{"required":["value"]}}`,
		`{"type":"object","properties":{"value":{"type":"string"}},"allOf":[{"maxProperties":0}]}`,
		`{"type":"object","properties":{"value":{"type":"string"}},"if":{"required":["value"]},"then":{"required":["other"]}}`,
	} {
		c := bodyValueCatalog(t, "3.1.0", schema, "application/x-www-form-urlencoded", true)
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader("value=a"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		got, err := c.ValidateRequest("POST /body", req)
		if err != nil || len(got.Issues) == 0 {
			t.Fatalf("whole object assertion skipped: %+v %v", got, err)
		}
	}
}

func TestURLEncodedObjectWithoutPropertyBindings(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object"}`,
		`{"type":"object","required":["value"]}`,
		`{"type":"object","additionalProperties":{"type":"integer"}}`,
		`{"type":"object","allOf":[{"properties":{"value":{"type":"string"}}}]}`,
	} {
		c := bodyValueCatalog(t, "3.1.0", schema, "application/x-www-form-urlencoded", true)
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader("value=a"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := c.ValidateRequest("POST /body", req); !errors.Is(err, ErrUnsupportedBodyEncoding) {
			t.Fatalf("undefined property binding must fail explicitly: %v", err)
		}
	}
}

func TestURLEncodedBodyPresenceCharsetAndBounds(t *testing.T) {
	c := bodyValueCatalog(t, "3.1.0", `{"type":"object","properties":{"value":{"type":"string"},"optional":{"type":"array"}},"minProperties":0}`, "application/x-www-form-urlencoded", true)
	for _, tc := range []struct {
		body, charset      string
		valid, unsupported bool
	}{
		{"", "", true, false},
		{"value=", "", true, false},
		{"value=%C3%A9", "utf-8", true, false},
		{"value=%C3%A9", "us-ascii", false, false},
		{"value=a", "iso-8859-1", false, true},
		{strings.Repeat("value=a&", MaxFormFields), "", false, false},
		{"value=" + strings.Repeat("a", MaxJSONBodyBytes), "", false, false},
	} {
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded"+func() string {
			if tc.charset == "" {
				return ""
			}
			return "; charset=" + tc.charset
		}())
		got, err := c.ValidateRequest("POST /body", req)
		if tc.unsupported {
			if !errors.Is(err, ErrUnsupportedBodyEncoding) {
				t.Fatalf("charset: %+v %v", got, err)
			}
		} else if err != nil || (len(got.Issues) == 0) != tc.valid {
			t.Fatalf("body bytes=%d charset=%q: %+v %v", len(tc.body), tc.charset, got, err)
		}
	}
}
