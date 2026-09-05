package catalog

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExplicitEmptyBodyContracts(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		for _, required := range []bool{false, true} {
			for _, tt := range []struct {
				name, media, schema string
				present, valid      bool
			}{
				{"empty text", "text/plain", `{"type":"string","enum":[""]}`, true, true},
				{"text minimum", "text/plain", `{"type":"string","minLength":1}`, true, false},
				{"text enum", "text/plain", `{"type":"string","enum":["value"]}`, true, false},
				{"text absent", "text/plain", `{"type":"string","minLength":1}`, false, !required},
				{"empty JSON", "application/json", `{"type":"object"}`, true, false},
				{"JSON absent", "application/json", `{"type":"object"}`, false, !required},
				{"empty binary", "application/octet-stream", `{"type":"string","format":"binary"}`, true, true},
				{"binary absent", "application/octet-stream", `{"type":"string","format":"binary"}`, false, !required},
				{"empty XML decoder", "application/xml", `{"type":"object"}`, true, false},
			} {
				t.Run(fmt.Sprintf("%s/required=%t/%s", version, required, tt.name), func(t *testing.T) {
					c := bodyValueCatalog(t, version, tt.schema, tt.media, required)
					var body io.Reader
					if tt.present {
						body = strings.NewReader("")
					}
					req, _ := http.NewRequest("POST", "http://gateway.test/body", body)
					req.Header.Set("Content-Type", tt.media)
					checked, err := c.ValidateRequest("POST /body", req)
					if (err == nil && len(checked.Issues) == 0) != tt.valid {
						t.Fatalf("valid=%t present=%t: %+v %v", tt.valid, tt.present, checked, err)
					}
				})
			}
		}
	}
}

func TestExplicitEmptyBodyRequiresDeclarationAndMedia(t *testing.T) {
	const without = `{"openapi":"3.1.0","info":{"title":"No body","version":"test"},"paths":{"/body":{"post":{"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(without))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	declared := bodyValueCatalog(t, "3.1.0", `{"type":"string"}`, "text/plain", false)
	for _, selected := range []*Catalog{c, declared} {
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(""))
		checked, err := selected.ValidateRequest("POST /body", req)
		if err == nil && len(checked.Issues) == 0 {
			t.Fatal("explicit empty body bypassed its declaration or media type")
		}
	}
}
