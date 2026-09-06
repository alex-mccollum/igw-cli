package catalog

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const discoverySpec = `{"openapi":"3.1.0","info":{"title":"Discovery","version":"1"},"security":[{"Token":[]}],"paths":{"/items/{id}":{"parameters":[{"$ref":"#/components/parameters/ID"}],"get":{"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/A"}}}}}},"delete":{"responses":{"204":{"description":"deleted"}}}}},"components":{"securitySchemes":{"Token":{"type":"apiKey","in":"header","name":"X-Key"},"Unused":{"type":"http","scheme":"bearer"}},"parameters":{"ID":{"in":"path","name":"id","required":true,"schema":{"type":"integer"}}},"schemas":{"A":{"type":"object","properties":{"b":{"$ref":"#/components/schemas/B"}}},"B":{"type":"array","items":{"$ref":"#/components/schemas/A"}},"Unused":{"type":"string"}}}}`

func TestCompactDescriptionClosesDependencies(t *testing.T) {
	c, err := Parse([]byte(discoverySpec))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.DescribeCompact("GET /items/{id}")
	if err != nil {
		t.Fatal(err)
	}
	var item map[string]json.RawMessage
	var components map[string]map[string]json.RawMessage
	if json.Unmarshal(d.PathItem, &item) != nil || json.Unmarshal(d.Components, &components) != nil {
		t.Fatal("invalid compact JSON")
	}
	if len(item) != 2 || item["get"] == nil || item["parameters"] == nil || len(components["schemas"]) != 2 || components["schemas"]["A"] == nil || components["schemas"]["B"] == nil || len(components["securitySchemes"]) != 1 || components["securitySchemes"]["Token"] == nil || components["parameters"]["ID"] == nil || d.Document != nil {
		t.Fatalf("incomplete or bloated closure: %+v", d)
	}
	full, err := c.Describe("GET /items/{id}")
	if err != nil || !bytes.Contains(full.PathItem, []byte(`"delete"`)) || !bytes.Contains(full.Components, []byte(`"Unused"`)) {
		t.Fatal("full discovery was pruned or catalog mutated")
	}
}

func TestCompactDescriptionHonorsOperationSecurity(t *testing.T) {
	raw := strings.Replace(discoverySpec, `"get":{"responses"`, `"get":{"security":[],"responses"`, 1)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.DescribeCompact("GET /items/{id}")
	if err != nil || string(d.Security) != "[]" || bytes.Contains(d.Components, []byte(`"securitySchemes"`)) {
		t.Fatal("operation override retained unrelated authentication requirements")
	}
}

func TestCompactDescriptionRetainsDynamicScope(t *testing.T) {
	raw := strings.Replace(discoverySpec, `"B":{"type":"array","items":{"$ref":"#/components/schemas/A"}}`, `"B":{"$id":"urn:igw:tree","$dynamicAnchor":"node","type":"array","items":{"$dynamicRef":"#node"}}`, 1)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.DescribeCompact("GET /items/{id}")
	if err != nil || !bytes.Equal(d.Document, []byte(raw)) {
		t.Fatalf("dynamic reference lost containing document: %v", err)
	}
}
