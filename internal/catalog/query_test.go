package catalog

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func changedFilterCatalog(t *testing.T, change func(map[string]any)) *Catalog {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(queryFilterSpec), &doc); err != nil {
		t.Fatal(err)
	}
	item := doc["paths"].(map[string]any)["/items"].(map[string]any)
	change(item)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

const queryFilterSpec = `{
 "openapi":"3.1.0","info":{"title":"Synthetic filter API","version":"test"},
 "paths":{"/items":{"get":{
  "parameters":[
   {"name":"filter","in":"query","style":"form","explode":true,"allowReserved":true,
    "schema":{"type":"object","minProperties":1,"maxProperties":2,
     "propertyNames":{"pattern":"^[a-z]+\\[(eq|gt)\\]$"},
     "properties":{"value[eq]":{"type":"string","const":"true"}},
     "additionalProperties":{"oneOf":[{"type":"string","minLength":1},{"type":"number"},{"type":"boolean"}]}}},
   {"name":"name","in":"query","required":true,"schema":{"type":"string"}},
   {"name":"limit","in":"query","schema":{"type":"integer","maximum":100}}
  ],"responses":{"200":{"description":"OK"}}
 }}}
}`

func TestExplodedFilterBindingPreservesPeersAndValues(t *testing.T) {
	c, err := Parse([]byte(queryFilterSpec))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tt := range []struct {
		name, query string
		valid       bool
	}{
		{"absent optional object", "name=present&limit=10", true},
		{"one property", "name=present&field%5Beq%5D=one", true},
		{"filter and scalar", "name=present&limit=10&field%5Beq%5D=one", true},
		{"bracket property is not peer name", "name=present&name%5Beq%5D=other", true},
		{"literal primitive remains string", "name=present&value%5Beq%5D=true", true},
		{"reserved characters", "name=present&field%5Beq%5D=a%2Bb%26%23%25%3F%2F", true},
		{"missing scalar", "field%5Beq%5D=one", false},
		{"filter cannot supply missing scalar", "name%5Beq%5D=one", false},
		{"invalid scalar after filter", "name=present&limit=101&field%5Beq%5D=one", false},
		{"object constraint", "name=present&a%5Beq%5D=one&b%5Beq%5D=two&c%5Beq%5D=three", false},
		{"property constraint", "name=present&value%5Beq%5D=false", false},
		{"empty value constraint", "name=present&field%5Beq%5D=", false},
		{"unknown operator", "name=present&field%5Bunknown%5D=one", false},
		{"unknown property", "name=present&unexpected=one", false},
		{"named object is not exploded", "name=present&filter=one", false},
		{"duplicate property", "name=present&field%5Beq%5D=one&field%5Beq%5D=two", false},
		{"malformed encoding", "name=present&field%5Beq%5D=%zz", false},
		{"unescaped semicolon", "name=present&field%5Beq%5D=a;b", false},
		{"model remains reusable", "name=present&limit=10", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://gateway.test/items?"+tt.query, nil)
			if err != nil {
				t.Fatal(err)
			}
			issues, err := c.Validate("GET /items", req)
			if err != nil || (len(issues) == 0) != tt.valid {
				t.Fatalf("valid=%t: issues=%+v err=%v", tt.valid, issues, err)
			}
			if req.URL.RawQuery != tt.query || string(c.Raw()) != queryFilterSpec {
				t.Fatal("validation changed the outgoing request or vendor document")
			}
		})
	}
}

func TestExplodedFilterRejectsAmbiguousOwnership(t *testing.T) {
	raw := strings.Replace(queryFilterSpec, `"name":"name"`, `"name":"field[eq]"`, 1)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req, _ := http.NewRequest("GET", "http://gateway.test/items?field%5Beq%5D=one", nil)
	issues, err := c.Validate("GET /items", req)
	if err != nil || len(issues) == 0 {
		t.Fatal("accepted a key claimed by both filter and scalar parameter")
	}
}

func TestExplodedFilterInheritanceAndRequestBody(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE"} {
		t.Run(method, func(t *testing.T) {
			c := changedFilterCatalog(t, func(item map[string]any) {
				op := item["get"].(map[string]any)
				params := op["parameters"].([]any)
				item["parameters"] = []any{params[0], map[string]any{"in": "header", "name": "X-Test", "required": true, "schema": map[string]any{"type": "string"}}}
				op["parameters"] = params[1:]
				if method == "POST" || method == "PUT" || method == "PATCH" {
					op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"enabled"}, "properties": map[string]any{"enabled": map[string]any{"type": "boolean"}}}}}}
				}
				delete(item, "get")
				item[strings.ToLower(method)] = op
			})
			for _, header := range []bool{false, true} {
				body := ""
				if method == "POST" || method == "PUT" || method == "PATCH" {
					body = `{"enabled":true}`
				}
				req, _ := http.NewRequest(method, "http://gateway.test/items?name=present&field%5Beq%5D=one", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				if header {
					req.Header.Set("X-Test", "present")
				}
				issues, err := c.Validate(method+" /items", req)
				if err != nil || (len(issues) == 0) != header {
					t.Fatalf("inherited filter/header: %+v %v", issues, err)
				}
			}
			if method == "POST" || method == "PUT" || method == "PATCH" {
				for _, body := range []string{"", `{}`, `{"enabled":"wrong"}`} {
					req, _ := http.NewRequest(method, "http://gateway.test/items?name=present&field%5Beq%5D=one", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Test", "present")
					issues, err := c.Validate(method+" /items", req)
					if err != nil || len(issues) == 0 {
						t.Fatalf("filter view lost body validation: %+v %v", issues, err)
					}
				}
			}
		})
	}
}

func TestExplodedFilterRequiredAndOverriddenDeclarations(t *testing.T) {
	for _, override := range []bool{false, true} {
		c := changedFilterCatalog(t, func(item map[string]any) {
			op := item["get"].(map[string]any)
			params := op["parameters"].([]any)
			inherited := make(map[string]any)
			for k, v := range params[0].(map[string]any) {
				inherited[k] = v
			}
			inherited["required"] = true
			item["parameters"] = []any{inherited}
			if !override {
				op["parameters"] = params[1:]
			}
		})
		for _, supplied := range []bool{false, true} {
			query := "name=present"
			if supplied {
				query += "&field%5Beq%5D=one"
			}
			req, _ := http.NewRequest("GET", "http://gateway.test/items?"+query, nil)
			issues, err := c.Validate("GET /items", req)
			if err != nil || (len(issues) == 0) != (override || supplied) {
				t.Fatalf("required/override semantics: %+v %v", issues, err)
			}
		}
	}
}

func TestExplodedFilterRejectsDuplicateDeclarations(t *testing.T) {
	c := changedFilterCatalog(t, func(item map[string]any) {
		op := item["get"].(map[string]any)
		params := op["parameters"].([]any)
		op["parameters"] = append(params, map[string]any{"name": "name", "in": "query", "schema": map[string]any{"type": "string"}})
	})
	req, _ := http.NewRequest("GET", "http://gateway.test/items?name=present&field%5Beq%5D=one", nil)
	issues, err := c.Validate("GET /items", req)
	if err != nil || len(issues) != 1 || issues[0].Rule != "ambiguous_parameter_binding" {
		t.Fatalf("duplicate declarations: %+v %v", issues, err)
	}
}

func TestExplodedFilterConcurrentValidation(t *testing.T) {
	c, err := Parse([]byte(queryFilterSpec))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 5 {
				const query = "name=present&limit=10&name%5Beq%5D=other"
				req, _ := http.NewRequest("GET", "http://gateway.test/items?"+query, nil)
				issues, err := c.Validate("GET /items", req)
				if err != nil || len(issues) != 0 || req.URL.RawQuery != query {
					t.Errorf("concurrent filter validation: %+v %v", issues, err)
					return
				}
			}
		}()
	}
	workers.Wait()
}
