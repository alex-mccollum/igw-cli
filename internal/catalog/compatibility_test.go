package catalog

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

// Reduced synthetic fixture for defects observed in official 8.3 captures.
const compatibilityFixture = `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1.0.0","license":{"url":"https://inductiveautomation.com/ignition/license","name":"Inductive Automation EULA"}},"paths":{"/items/{name}":{"get":{"parameters":[{"in":"path","name":"name","required":true,"allowReserved":false,"schema":{"type":"string"}}],"responses":{}}}}}`

func TestCompatibilityPreservesRawAndExposesGaps(t *testing.T) {
	t.Parallel()
	c, err := Parse([]byte(compatibilityFixture))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !bytes.Equal(c.Raw(), []byte(compatibilityFixture)) {
		t.Fatal("vendor evidence was rewritten")
	}
	compat := c.Compatibility()
	if compat == nil || compat.Adjustments != 2 || compat.Rules["empty-responses"] != 1 {
		t.Fatalf("missing compatibility evidence: %+v", compat)
	}
	d, err := c.Describe("GET /items/{name}")
	if err != nil || len(d.Adjustments) != 2 || len(d.Gaps) != 1 || !bytes.Contains(d.Operation.Definition, []byte(`"responses":{}`)) {
		t.Fatalf("description invented vendor contract: %+v %v", d, err)
	}
	compat.Rules["empty-responses"] = 99
	if c.Compatibility().Rules["empty-responses"] != 1 {
		t.Fatal("compatibility mutated through caller")
	}
	for _, raw := range []string{
		strings.Replace(compatibilityFixture, `"allowReserved":false`, `"allowReserved":true`, 1),
		strings.Replace(compatibilityFixture, "Ignition HTTP API", "Other API", 1),
		strings.Replace(compatibilityFixture, `"required":true`, `"required":false`, 1),
	} {
		if got, err := Parse([]byte(raw)); err == nil {
			got.Close()
			t.Fatal("adapter accepted an unreviewed defect")
		}
	}
}

func TestRequiredRecursiveArrayAcceptsFiniteTreesAndValidatesLeaves(t *testing.T) {
	t.Parallel()
	raw := `{"openapi":"3.1.0","info":{"title":"Recursive fixture","version":"test"},"paths":{"/tree":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Node"}}}},"responses":{"200":{"description":"OK"}}}}},"components":{"schemas":{"Node":{"type":"object","required":["name","children"],"properties":{"name":{"type":"string"},"children":{"type":"array","items":{"$ref":"#/components/schemas/Node"}}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"name":"root","children":[{"name":"leaf","children":[]}]}`, true},
		{`{"name":"root","children":[{"name":42,"children":[]}]}`, false},
	} {
		req, _ := http.NewRequest("POST", "http://gateway.test/tree", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("POST /tree", req)
		if err != nil || (len(issues) == 0) != tc.valid {
			t.Fatalf("recursive validation: %v %v", issues, err)
		}
	}
}
