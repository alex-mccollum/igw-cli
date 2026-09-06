package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDocumentValidationAgreesWithUpstream(t *testing.T) {
	for _, version := range []string{"3.0.1", "3.1.0"} {
		for _, tc := range []struct {
			name, members string
			valid         bool
		}{
			{"minimal", `"info":{"title":"test","version":"1"},"paths":{}`, true},
			{"missing info", `"paths":{}`, false},
			{"wrong title type", `"info":{"title":42,"version":"1"},"paths":{}`, false},
			{"wrong paths type", `"info":{"title":"test","version":"1"},"paths":[]`, false},
			{"unknown root field", `"info":{"title":"test","version":"1"},"paths":{},"surprise":true`, false},
			{"extension with exact integer", `"info":{"title":"test","version":"1"},"paths":{},"x-values":[9007199254740993,1e300,-1e-300]`, true},
			{"overflow extension", `"info":{"title":"test","version":"1"},"paths":{},"x-number":1e400`, false},
			{"missing response description", `"info":{"title":"test","version":"1"},"paths":{"/items":{"get":{"responses":{"200":{}}}}}`, false},
			{"parameter without name", `"info":{"title":"test","version":"1"},"paths":{"/items":{"get":{"parameters":[{"in":"query","schema":{"type":"string"}}],"responses":{"200":{"description":"ok"}}}}}`, false},
			{"valid referenced schema", `"info":{"title":"test","version":"1"},"paths":{"/items":{"get":{"responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Item"}}}}}}}},"components":{"schemas":{"Item":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993}}}}}`, true},
		} {
			t.Run(version+"/"+tc.name, func(t *testing.T) {
				raw := []byte(`{"openapi":"` + version + `",` + tc.members + `}`)
				decoder := json.NewDecoder(strings.NewReader(string(raw)))
				decoder.UseNumber()
				value, err := decodeUniqueJSON(decoder, 0)
				if err != nil {
					t.Fatal(err)
				}
				got := validDocumentValue(value, version)
				if got != tc.valid {
					t.Fatalf("direct=%t expected=%t", got, tc.valid)
				}
			})
		}
	}
}

func TestParseBoundsDocumentNumericWork(t *testing.T) {
	for _, number := range []string{"1e-100000000", "1e100000000", "1" + strings.Repeat("0", maxParameterNumberChars)} {
		raw := []byte(`{"openapi":"3.1.0","info":{"title":"test","version":"1"},"paths":{},"x-number":` + number + `}`)
		if c, err := Parse(raw); err == nil {
			c.Close()
			t.Fatal("unbounded document number accepted")
		}
	}
}
