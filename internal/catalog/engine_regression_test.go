package catalog

import (
	"net/http"
	"strings"
	"testing"
)

func TestEnginePathReferenceSiblingOverrides(t *testing.T) {
	const raw = `{"openapi":"3.1.0","info":{"title":"test","version":"1"},"paths":{"/base":{"get":{"operationId":"base","parameters":[{"in":"query","name":"value","schema":{"type":"integer","minimum":1}}],"responses":{"200":{"description":"OK"}}}},"/alias":{"$ref":"#/paths/~1base","get":{"operationId":"alias","parameters":[{"in":"query","name":"value","schema":{"type":"integer","minimum":3}}],"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	op, err := c.Resolve("alias")
	if err != nil || op.Key != "GET /alias" {
		t.Fatalf("alias metadata: %+v %v", op, err)
	}
	for _, tc := range []struct {
		query string
		valid bool
	}{{"2", false}, {"3", true}} {
		req, _ := http.NewRequest("GET", "http://gateway.test/alias?value="+tc.query, nil)
		issues, err := c.Validate(op.Key, req)
		if err != nil || (len(issues) == 0) != tc.valid {
			t.Fatalf("override: %v %v", issues, err)
		}
	}
}

func TestEngineIgnitionScopedComponentReference(t *testing.T) {
	const raw = `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1","license":{"name":"Inductive Automation EULA","url":"https://inductiveautomation.com/ignition/license"}},"paths":{"/body":{"post":{"requestBody":{"content":{"application/json":{"schema":{"$id":"ignition/example","type":"object","properties":{"value":{"$ref":"#/components/schemas/Value"}}}}}},"responses":{"200":{"description":"OK"}}}}},"components":{"schemas":{"Value":{"type":"integer","minimum":9007199254740993}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tc := range []struct {
		body  string
		valid bool
	}{{`{"value":9007199254740993}`, true}, {`{"value":9007199254740992}`, false}} {
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		issues, err := c.Validate("POST /body", req)
		if err != nil || (len(issues) == 0) != tc.valid {
			t.Fatalf("scoped component: %v %v", issues, err)
		}
	}
}

func TestEngineRequiredCookieCannotBeSilentlySkipped(t *testing.T) {
	const raw = `{"openapi":"3.1.0","info":{"title":"test","version":"1"},"paths":{"/value":{"get":{"parameters":[{"in":"cookie","name":"session","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req, _ := http.NewRequest("GET", "http://gateway.test/value", nil)
	issues, err := c.Validate("GET /value", req)
	if err != nil || len(issues) != 1 || issues[0].Parameter != "session" {
		t.Fatalf("missing cookie: %v %v", issues, err)
	}
}

func TestEngineValidationFieldPointers(t *testing.T) {
	const raw = `{"openapi":"3.1.0","info":{"title":"test","version":"1"},"paths":{"/body":{"post":{"requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"a/b":{"type":"object","properties":{"~1":{"type":"array","items":{"type":"integer"}}}},"":{"type":"integer"}}}}}},"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tc := range []struct {
		name, body, pointer string
	}{
		{"root", `[]`, ""},
		{"empty property", `{"":"invalid"}`, "/"},
		{"escaped nested array", `{"a/b":{"~1":["invalid"]}}`, "/a~1b/~01/0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			issues, err := c.Validate("POST /body", req)
			if err != nil || len(issues) != 1 || issues[0].Field != tc.pointer || issues[0].Kind != "requestBody" || issues[0].Rule != "schema" {
				t.Fatalf("field pointer: %+v %v; want %q", issues, err, tc.pointer)
			}
		})
	}
}
