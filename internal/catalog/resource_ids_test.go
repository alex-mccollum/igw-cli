package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const resourceIDVariant = `{"$id":"ignition/schedule/basic schedule","title":"basic schedule","type":"object","properties":{"enabled":{"type":"boolean"}},"required":["enabled"],"additionalProperties":false}`

func resourceIDFixture(primary, backup string) string {
	return `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1.0.0","license":{"url":"https://inductiveautomation.com/ignition/license","name":"EULA"}},"paths":{"/data/api/v1/resources/ignition/schedule":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","items":{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"config":{"properties":{"settings":{"oneOf":[` + primary + `]}}},"backupConfig":{"properties":{"settings":{"oneOf":[` + backup + `]}}}}}}}}},"responses":{"200":{"description":"OK"}}}}}}`
}

func TestResourceIDAdapterPreservesBothVariantConstraints(t *testing.T) {
	for _, method := range []string{"POST", "PUT"} {
		t.Run(method, func(t *testing.T) {
			raw := strings.ReplaceAll(resourceIDFixture(resourceIDVariant, resourceIDVariant), `"post":`, `"`+strings.ToLower(method)+`":`)
			c, err := Parse([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			path := "/data/api/v1/resources/ignition/schedule"
			d, err := c.Describe(method + " " + path)
			if err != nil || len(d.Adjustments) != 2 || !bytes.Equal(c.Raw(), []byte(raw)) || bytes.Count(d.Operation.Definition, []byte(`"$id"`)) != 2 {
				t.Fatal("adapter did not preserve original contract and evidence")
			}
			for _, tc := range []struct {
				body  string
				valid bool
			}{
				{`[{"name":"test","config":{"settings":{"enabled":true}},"backupConfig":{"settings":{"enabled":false}}}]`, true},
				{`[{"name":"test","config":{"settings":{"enabled":"invalid"}}}]`, false},
				{`[{"name":"test","backupConfig":{"settings":{"enabled":"invalid"}}}]`, false},
				{`[{"name":"test","config":{"settings":{}}}]`, false},
				{`[{"name":"test","config":{"settings":{"enabled":true,"extra":1}}}]`, false},
			} {
				req, _ := http.NewRequest(method, "http://gateway.test"+path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				issues, err := c.Validate(method+" "+path, req)
				if err != nil || (len(issues) == 0) != tc.valid {
					t.Fatalf("variant validation: %v %v", issues, err)
				}
			}
		})
	}
}

func TestResourceIDAdapterRefusesUnreviewedShapes(t *testing.T) {
	addKeyword := func(keyword string) string {
		return strings.Replace(resourceIDVariant, `"type":"object"`, keyword+`,"type":"object"`, 1)
	}
	mixedReferences := resourceIDVariant + "," + strings.ReplaceAll(addKeyword(`"$ref":"#/components/schemas/Other"`), "basic schedule", "another schedule")
	for _, tc := range []struct{ name, primary, backup string }{
		{"different-assertion", resourceIDVariant, strings.Replace(resourceIDVariant, `"boolean"`, `"string"`, 1)},
		{"reference", addKeyword(`"$ref":"#/components/schemas/Other"`), addKeyword(`"$ref":"#/components/schemas/Other"`)},
		{"nested-reference", strings.Replace(resourceIDVariant, `"type":"boolean"`, `"$ref":"#/components/schemas/Other"`, 1), strings.Replace(resourceIDVariant, `"type":"boolean"`, `"$ref":"#/components/schemas/Other"`, 1)},
		{"anchor", addKeyword(`"$anchor":"here"`), addKeyword(`"$anchor":"here"`)},
		{"dynamic", addKeyword(`"$dynamicRef":"#here"`), addKeyword(`"$dynamicRef":"#here"`)},
		{"dialect", addKeyword(`"$schema":"https://json-schema.org/draft/2020-12/schema"`), addKeyword(`"$schema":"https://json-schema.org/draft/2020-12/schema"`)},
		{"unknown-scope", addKeyword(`"$future":"here"`), addKeyword(`"$future":"here"`)},
		{"wrong-identifier", strings.Replace(resourceIDVariant, "ignition/schedule/", "other/", 1), strings.Replace(resourceIDVariant, "ignition/schedule/", "other/", 1)},
		{"nested-identifier", strings.Replace(resourceIDVariant, `"type":"boolean"`, `"$id":"other","type":"boolean"`, 1), strings.Replace(resourceIDVariant, `"type":"boolean"`, `"$id":"other","type":"boolean"`, 1)},
		{"all-or-nothing", mixedReferences, mixedReferences},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Avoid parsing intentionally invalid references/dialects. The adapter
			// must leave the entire private input unchanged before normal checks.
			raw := resourceIDFixture(tc.primary, tc.backup)
			value := identityValue(t, raw)
			before, _ := json.Marshal(value)
			adjustments := normalizeIgnition(value)
			after, _ := json.Marshal(value)
			if len(adjustments) != 0 || !bytes.Equal(before, after) {
				t.Fatal("unreviewed schema was rewritten")
			}
		})
	}
}

func TestResourceIDAdapterPreservesAlternativeOrderAndExclusivity(t *testing.T) {
	other := strings.ReplaceAll(resourceIDVariant, "basic schedule", "another schedule")
	other = strings.ReplaceAll(other, `"boolean"`, `"string"`)
	raw := resourceIDFixture(resourceIDVariant+","+other, other+","+resourceIDVariant)
	value := identityValue(t, raw)
	adjustments := normalizeIgnition(value)
	if len(adjustments) != 4 {
		t.Fatal("reordered duplicate pairs were not qualified")
	}
	// Every assertion and the original alternative order must remain intact.
	// In particular, the adapter must not weaken oneOf to anyOf.
	expected := strings.ReplaceAll(raw, `"$id":"ignition/schedule/basic schedule",`, "")
	expected = strings.ReplaceAll(expected, `"$id":"ignition/schedule/another schedule",`, "")
	want, _ := json.Marshal(identityValue(t, expected))
	got, _ := json.Marshal(value)
	if !bytes.Equal(got, want) {
		t.Fatal("adapter changed more than the unused identifiers")
	}
}
