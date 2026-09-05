package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Synthetic fixture: this exercises OpenAPI contracts, not Ignition compatibility.
const testSpec = `{
 "openapi":"3.1.0","info":{"title":"Synthetic test API","version":"test"},
 "paths":{
   "/items/{name}":{
     "parameters":[{"name":"name","in":"path","required":true,"schema":{"type":"string"}}],
     "get":{"operationId":"item","responses":{"200":{"description":"OK"}}},
     "put":{"operationId":"item","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Item"}}}},"responses":{"200":{"description":"OK"}}}
   },
   "/health":{"get":{"responses":{"200":{"description":"OK"}}}}
 },
 "components":{"schemas":{"Item":{"type":"object","required":["enabled"],"additionalProperties":false,"properties":{"enabled":{"type":"boolean"},"count":{"type":"integer","minimum":9007199254740993}}}}}
}`

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := Parse([]byte(testSpec))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestCatalogKeepsCompleteContractAndUnambiguousKeys(t *testing.T) {
	t.Parallel()
	c := testCatalog(t)
	if len(c.Operations()) != 3 {
		t.Fatalf("operations: %v", c.Operations())
	}
	if _, err := c.Resolve("item"); err == nil {
		t.Fatal("ambiguous alias resolved")
	}
	if _, err := c.Resolve("GET /health"); err != nil {
		t.Fatal(err)
	}
	d, err := c.Describe("PUT /items/{name}")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		raw  []byte
		want string
	}{
		{d.Operation.Definition, "requestBody"}, {d.PathItem, "parameters"}, {d.Components, "9007199254740993"},
	} {
		if !bytes.Contains(entry.raw, []byte(entry.want)) {
			t.Fatalf("lost %s: %s", entry.want, entry.raw)
		}
	}
	if string(c.Raw()) != testSpec {
		t.Fatal("vendor bytes changed")
	}
	ops := c.Operations()
	ops[0].Definition[0] = '!'
	if c.Operations()[0].Definition[0] == '!' {
		t.Fatal("caller modified catalog")
	}
}

func TestContractDigestIgnoresFormattingWithoutLosingNumbers(t *testing.T) {
	t.Parallel()
	c := testCatalog(t)
	var buf bytes.Buffer
	if err := json.Compact(&buf, c.Raw()); err != nil {
		t.Fatal(err)
	}
	compact, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer compact.Close()
	if compact.RawHash() == c.RawHash() || compact.ContractHash() != c.ContractHash() {
		t.Fatal("incorrect canonical digest")
	}
	changed, err := Parse([]byte(strings.ReplaceAll(testSpec, "9007199254740993", "9007199254740992")))
	if err != nil {
		t.Fatal(err)
	}
	defer changed.Close()
	if changed.ContractHash() == c.ContractHash() {
		t.Fatal("large number change was lost")
	}
}

func TestCatalogRejectsInvalidAndExternalReferences(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	for _, raw := range []string{
		`<html>Gateway login</html>`, `{}`, testSpec + `{}`,
		strings.ReplaceAll(testSpec, "#/components/schemas/Item", "#/components/schemas/Missing"),
		strings.ReplaceAll(testSpec, "#/components/schemas/Item", srv.URL+"/schema.json"),
		strings.ReplaceAll(testSpec, "#/components/schemas/Item", "file:///private/secret"),
		strings.Replace(testSpec, `"type":"object"`, `"$schema":"`+srv.URL+`/dialect","type":"object"`, 1),
		strings.Replace(testSpec, `"openapi":"3.1.0"`, `"openapi":"3.1.0","openapi":"3.0.3"`, 1),
	} {
		if c, err := Parse([]byte(raw)); err == nil {
			c.Close()
			t.Fatal("invalid document was accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("external reference fetched")
	}
}

func TestCatalogValidatesBodyAndDoesNotExposeValues(t *testing.T) {
	t.Parallel()
	c := testCatalog(t)
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"enabled":true}`, true}, {`{}`, false}, {`{"enabled":"private-secret"}`, false},
	} {
		req, err := http.NewRequest("PUT", "http://gateway.test/items/demo", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("PUT /items/{name}", req)
		if err != nil {
			t.Fatal(err)
		}
		if (len(issues) == 0) != tc.valid {
			t.Fatalf("body %s: issues %v", tc.body, issues)
		}
		b, _ := json.Marshal(issues)
		if bytes.Contains(b, []byte("private-secret")) {
			t.Fatal("validation leaked instance values")
		}
	}
}

func TestSCIMReferencePropertyIsDataButItsSchemaIsChecked(t *testing.T) {
	t.Parallel()
	raw := `{"openapi":"3.1.0","info":{"title":"SCIM regression","version":"test"},"paths":{"/groups":{"post":{"requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"$ref":{"type":"string"}}}}}},"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	for _, schema := range []string{`{"$ref":"https://example.invalid/schema"}`, `{"$schema":"https://example.invalid/dialect","type":"string"}`} {
		bad := strings.Replace(raw, `{"type":"string"}`, schema, 1)
		if c, err := Parse([]byte(bad)); err == nil {
			c.Close()
			t.Fatal("property's schema bypassed external reference protection")
		}
	}
}

func TestCatalogRejectsAmbiguousOrExcessivelyNestedJSON(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"openapi":"3.1.0","info":{"title":"A","title":"B","version":"test"},"paths":{}}`,
		`{"openapi":"3.1.0","info":{"title":"A","version":"test"},"paths":{},"x-deep":` + strings.Repeat("[", 257) + `0` + strings.Repeat("]", 257) + `}`,
	} {
		if c, err := Parse([]byte(raw)); err == nil {
			c.Close()
			t.Fatal("ambiguous or excessively nested input accepted")
		}
	}
}

func TestReferencedPathItemKeepsOperationDefinition(t *testing.T) {
	t.Parallel()
	raw := `{"openapi":"3.1.0","info":{"title":"Fixture","version":"test"},"paths":{"/health":{"$ref":"#/components/pathItems/Health"}},"components":{"pathItems":{"Health":{"get":{"operationId":"health","responses":{"200":{"description":"OK"}}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d, err := c.Describe("health")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(d.Operation.Definition, []byte("responses")) || !bytes.Contains(d.Components, []byte("pathItems")) {
		t.Fatalf("lost referenced operation: %+v", d)
	}
}

func TestTargetScopesProfileOriginAndProxyPath(t *testing.T) {
	t.Parallel()
	a, err := NewTarget("dev", "https://GATEWAY.test:443/proxy/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewTarget("dev", "https://gateway.test/proxy")
	if a != b || a.Key() != b.Key() {
		t.Fatal("equivalent targets differ")
	}
	endpoint, err := a.Endpoint("/openapi.json")
	if err != nil || endpoint != "https://gateway.test/proxy/openapi.json" {
		t.Fatalf("proxy path lost: %s %v", endpoint, err)
	}
	for _, tc := range []struct{ profile, url string }{
		{"other", a.URL}, {"dev", "https://gateway.test/other"}, {"dev", "http://gateway.test/proxy"},
	} {
		other, _ := NewTarget(tc.profile, tc.url)
		if other.Key() == a.Key() {
			t.Fatal("target cache collision")
		}
	}
	for _, path := range []string{"https://foreign.test/openapi.json", "//foreign.test", "/../escape", "/%2e%2e/escape", "/x?token=secret"} {
		if _, err := a.Endpoint(path); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", path)
		}
	}
}
