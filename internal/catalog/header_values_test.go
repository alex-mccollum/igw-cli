package catalog

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func headerValueCatalog(t *testing.T, version, parameter string) *Catalog {
	t.Helper()
	raw := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Header inputs","version":"test"},"paths":{"/headers":{"get":{"parameters":[%s],"responses":{"200":{"description":"OK"}}}}}}`, version, parameter)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestExactHeaderPrimitiveValues(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, tt := range []struct {
			name, schema string
			values       []string
			valid        bool
		}{
			{"empty present", `{"type":"string","maxLength":0}`, []string{""}, true},
			{"missing", `{"type":"string"}`, nil, false},
			{"Unicode whitespace", `{"type":"string","enum":["\u00a0text\u00a0"]}`, []string{"\u00a0text\u00a0"}, true},
			{"literal delimiters", `{"type":"string","enum":["a,b%2Cc"]}`, []string{"a,b%2Cc"}, true},
			{"one scalar", `{"type":"string","enum":["first"]}`, []string{"first", "second"}, false},
			{"integer beyond int64", `{"type":"integer","minimum":0}`, []string{"18446744073709551616"}, true},
			{"integer minimum", `{"type":"integer","minimum":2}`, []string{"1"}, false},
			{"integer spelling", `{"type":"integer"}`, []string{"+1"}, false},
			{"precise maximum", `{"type":"number","maximum":9007199254740993}`, []string{"9007199254740993"}, true},
			{"precise minimum", `{"type":"number","minimum":9007199254740993}`, []string{"9007199254740992"}, false},
			{"decimal multiple", `{"type":"number","multipleOf":0.1}`, []string{"0.300000000000000000001"}, false},
			{"nonfinite", `{"type":"number"}`, []string{"NaN"}, false},
			{"boolean enum", `{"type":"boolean","enum":[false]}`, []string{"true"}, false},
			{"boolean value", `{"type":"boolean","enum":[false]}`, []string{"false"}, true},
			{"boolean alias", `{"type":"boolean"}`, []string{"1"}, false},
			{"boolean case", `{"type":"boolean"}`, []string{"TRUE"}, false},
			{"invalid UTF8", `{"type":"string"}`, []string{"\xff"}, false},
		} {
			t.Run(version+"/"+tt.name, func(t *testing.T) {
				c := headerValueCatalog(t, version, fmt.Sprintf(`{"in":"header","name":"X-Value","required":true,"schema":%s}`, tt.schema))
				req, _ := http.NewRequest("GET", "http://gateway.test/headers", nil)
				req.Header["X-Value"] = tt.values
				before := req.Header.Clone()
				issues, err := c.Validate("GET /headers", req)
				if err != nil || (len(issues) == 0) != tt.valid {
					t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
				}
				if !reflect.DeepEqual(req.Header, before) {
					t.Fatal("header validation changed caller input")
				}
			})
		}
	}
}

func TestExactHeaderArrayValues(t *testing.T) {
	for _, explode := range []bool{false, true} {
		for _, tt := range []struct {
			schema string
			values []string
			valid  bool
		}{
			{`{"type":"array","minItems":2,"items":{"type":"integer","minimum":0}}`, []string{"1", "18446744073709551616"}, true},
			{`{"type":"array","maxItems":2,"items":{"type":"string"}}`, []string{"one,two", "three"}, false},
			{`{"type":"array","uniqueItems":true,"items":{"type":"string"}}`, []string{"one", "one"}, false},
			{`{"type":"array","items":{"type":"string","minLength":2}}`, []string{"a,b"}, false},
			{`{"type":"array","items":{"type":"boolean","enum":[false]}}`, []string{"false,true"}, false},
			{`{"type":"array","minItems":2,"items":{"type":"number","multipleOf":0.1}}`, []string{"0.3, 0.4"}, true},
			{`{"type":"array","items":{"type":"number","multipleOf":0.1}}`, []string{"0.3,0.300000000000000000001"}, false},
			{`{"type":"array","items":{"type":"string"}}`, []string{""}, false},
			{`{"type":"array","items":{"type":"string"}}`, []string{"one,,two"}, false},
			{`{"type":"array","items":{"type":"string","enum":["a%2Cb","\"quoted\"","\u00a0text\u00a0"]}}`, []string{"a%2Cb, \"quoted\"", "\u00a0text\u00a0"}, true},
		} {
			t.Run(fmt.Sprintf("explode=%t/%s", explode, tt.schema), func(t *testing.T) {
				c := headerValueCatalog(t, "3.1.0", fmt.Sprintf(`{"in":"header","name":"X-Value","required":true,"style":"simple","explode":%t,"schema":%s}`, explode, tt.schema))
				req, _ := http.NewRequest("GET", "http://gateway.test/headers", nil)
				req.Header["X-Value"] = tt.values
				issues, err := c.Validate("GET /headers", req)
				if err != nil || (len(issues) == 0) != tt.valid {
					t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
				}
			})
		}
	}
}

func TestExactHeaderContentValues(t *testing.T) {
	for _, tt := range []struct {
		name, media, schema string
		values              []string
		valid               bool
	}{
		{"JSON nested number", "application/json", `{"type":"object","properties":{"value":{"type":"integer","minimum":9007199254740993}},"required":["value"]}`, []string{`{"value":9007199254740992}`}, false},
		{"JSON exact value", "application/json", `{"type":"integer","const":9007199254740993}`, []string{"9007199254740993"}, true},
		{"JSON enum", "application/json", `{"type":"boolean","enum":[false]}`, []string{"true"}, false},
		{"JSON null allowed", "application/json", `{"type":"null"}`, []string{"null"}, true},
		{"JSON null refused", "application/json", `{"type":"object"}`, []string{"null"}, false},
		{"JSON false schema", "application/json", `false`, []string{"null"}, false},
		{"JSON missing", "application/json", `true`, nil, false},
		{"JSON duplicate", "application/json", `{"type":"object"}`, []string{`{"value":1,"value":2}`}, false},
		{"JSON empty", "application/json", `{"type":"string"}`, []string{""}, false},
		{"JSON trailing", "application/json", `{"type":"object"}`, []string{`{} {}`}, false},
		{"JSON multiple fields", "application/json", `{"type":"object"}`, []string{`{}`, `{}`}, false},
		{"JSON invalid Unicode", "application/json", `{"type":"string"}`, []string{`"\ud800"`}, false},
		{"JSON string spaces", "application/json", `{"type":"string","enum":[" exact "]}`, []string{`" exact "`}, true},
		{"JSON empty array", "application/json", `{"type":"array","maxItems":0}`, []string{`[]`}, true},
		{"JSON suffix", "application/example+json", `{"type":"array","items":{"type":"string"}}`, []string{`["", "a,b", "\"quoted\""]`}, true},
		{"JSON numeric limit", "application/json", `{"type":"number"}`, []string{`1e4097`}, false},
		{"JSON ASCII charset", "application/json; charset=us-ascii", `{"type":"string"}`, []string{`"日本"`}, false},
		{"text empty", "text/plain", `{"type":"string","maxLength":0}`, []string{""}, true},
		{"text constraints", "text/plain", `{"type":"string","minLength":2}`, []string{"a"}, false},
		{"text charset", "text/plain; charset=utf-8", `{"type":"string","enum":["日本"]}`, []string{"日本"}, true},
		{"text unsupported charset", "text/plain; charset=iso-8859-1", `{"type":"string"}`, []string{"text"}, false},
		{"unsupported XML", "application/xml", `{"type":"object"}`, []string{"<value/>"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := headerValueCatalog(t, "3.1.0", fmt.Sprintf(`{"in":"header","name":"X-Value","required":true,"content":{%q:{"schema":%s}}}`, tt.media, tt.schema))
			req, _ := http.NewRequest("GET", "http://gateway.test/headers", nil)
			req.Header["X-Value"] = tt.values
			issues, err := c.Validate("GET /headers", req)
			if (err == nil && len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
			}
		})
	}
}

func TestHeaderEffectiveBinding(t *testing.T) {
	for _, tt := range []struct {
		name, parameter string
		headers         http.Header
		valid           bool
	}{
		{"case and whitespace", `{"in":"header","name":"x-value","schema":{"type":"string","enum":["exact"]}}`, http.Header{"x-value": {" \texact\t "}}, true},
		{"case variants scalar", `{"in":"header","name":"X-Value","schema":{"type":"string"}}`, http.Header{"X-Value": {"one"}, "x-value": {"two"}}, false},
		{"case variants list", `{"in":"header","name":"X-Value","schema":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"string"}}}`, http.Header{"X-Value": {"one"}, "x-value": {"two"}}, true},
		{"case declaration collision", `{"in":"header","name":"X-Value","schema":{"type":"string"}},{"in":"header","name":"x-value","schema":{"type":"integer"}}`, http.Header{"X-Value": {"1"}}, false},
		{"optional unsupported omitted", `{"in":"header","name":"X-Value","content":{"application/xml":{"schema":{"type":"string"}}}}`, nil, true},
		{"optional unsupported present", `{"in":"header","name":"X-Value","content":{"application/xml":{"schema":{"type":"string"}}}}`, http.Header{"X-Value": {"<value/>"}}, false},
		{"invalid field", `{"in":"header","name":"X-Value","schema":{"type":"string"}}`, http.Header{"X-Value": {"private\x00text"}}, false},
		{"nil required", `{"in":"header","name":"X-Value","required":true,"schema":{"type":"string"}}`, http.Header{"X-Value": nil}, false},
		{"empty slice required", `{"in":"header","name":"X-Value","required":true,"schema":{"type":"string"}}`, http.Header{"X-Value": {}}, false},
		{"managed token present", `{"in":"header","name":"x-ignition-api-token","required":true,"schema":{"type":"string","enum":["never-use-this-as-a-token"]}}`, http.Header{"X-Ignition-API-Token": {"managed"}}, true},
		{"managed token absent", `{"in":"header","name":"X-Ignition-API-Token","required":true,"schema":{"type":"string"}}`, nil, false},
		{"generated user agent", `{"in":"header","name":"User-Agent","schema":{"type":"integer"}}`, nil, false},
		{"explicit user agent", `{"in":"header","name":"User-Agent","schema":{"type":"string","enum":["explicit"]}}`, http.Header{"User-Agent": {"explicit"}}, true},
		{"generated encoding", `{"in":"header","name":"Accept-Encoding","schema":{"type":"integer"}}`, nil, false},
		{"generated host", `{"in":"header","name":"Host","schema":{"type":"integer"}}`, nil, false},
		{"generated length", `{"in":"header","name":"Content-Length","schema":{"type":"integer"}}`, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := headerValueCatalog(t, "3.1.0", tt.parameter)
			req, _ := http.NewRequest("GET", "http://gateway.test/headers", nil)
			req.Header = tt.headers
			before := req.Header.Clone()
			issues, err := c.Validate("GET /headers", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t: %+v %v", tt.valid, issues, err)
			}
			if !reflect.DeepEqual(req.Header, before) {
				t.Fatal("validation changed caller headers")
			}
		})
	}
}

func TestExactHeaderContentReferences(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		raw := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Header references","version":"test"},"paths":{"/headers":{"get":{"parameters":[{"in":"header","name":"X-Value","required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Node"}}}}],"responses":{"200":{"description":"OK"}}}}},"components":{"schemas":{"Node":{"type":"object","required":["value"],"properties":{"value":{"type":"integer","minimum":9007199254740993},"child":{"$ref":"#/components/schemas/Node"}}}}}}`, version)
		c, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		for _, tt := range []struct {
			value string
			valid bool
		}{
			{`{"value":9007199254740993,"child":{"value":9007199254740993}}`, true},
			{`{"value":9007199254740993,"child":{"value":9007199254740992}}`, false},
			{`{"value":9007199254740993,"child":null}`, false},
			{`null`, false},
		} {
			req, _ := http.NewRequest("GET", "http://gateway.test/headers", nil)
			req.Header.Set("X-Value", tt.value)
			issues, err := c.Validate("GET /headers", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("version=%s valid=%t: %+v %v", version, tt.valid, issues, err)
			}
		}
	}
}

func TestHeaderInheritanceAndReservedNames(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE"} {
		raw := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Header inheritance","version":"test"},"paths":{"/headers":{"parameters":[{"in":"header","name":"X-Value","required":true,"schema":{"type":"integer","minimum":2}}],%q:{"parameters":[{"in":"header","name":"x-value","required":true,"schema":{"type":"integer","minimum":0}},{"in":"header","name":"Accept","required":true,"schema":{"type":"integer"}},{"in":"header","name":"Content-Type","required":true,"schema":{"type":"integer"}},{"in":"header","name":"Authorization","required":true,"schema":{"type":"integer"}}],"responses":{"200":{"description":"OK"}}}}}}`, strings.ToLower(method))
		c, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer c.Close()
			req, _ := http.NewRequest(method, "http://gateway.test/headers", nil)
			req.Header.Set("X-Value", "1")
			issues, err := c.Validate(method+" /headers", req)
			if err != nil || len(issues) != 0 {
				t.Errorf("%s override/reserved declarations: %+v %v", method, issues, err)
			}
		}()
	}
}
