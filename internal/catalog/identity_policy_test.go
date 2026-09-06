package catalog

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestKeyboardIdentitySeparatesPolicyAndPreservesAssertions(t *testing.T) {
	for _, kind := range []string{"POST", "PUT", "find", "list"} {
		t.Run(kind, func(t *testing.T) {
			root := keyboardFixture(t, kind)
			paths := object(root["paths"])
			schema := map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "number"}}, "examples": []any{"old"}}
			paths["/status"] = map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{"schema": schema}}}}}}
			before, _ := json.Marshal(root)
			current, err := contractDigest(root)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(root)
			if !bytes.Equal(before, after) {
				t.Fatal("hashing mutated original evidence")
			}
			choices := schema["oneOf"].([]any)
			choices[0], choices[1] = choices[1], choices[0]
			schema["examples"] = []any{"new"}
			changedCurrent, _ := contractDigest(root)
			if changedCurrent != current {
				t.Fatal("reviewed reference scopes did not isolate documentation drift")
			}
			for path, rawItem := range paths {
				for method, rawOp := range object(rawItem) {
					pair := reviewedKeyboardDefinitions(object(rawOp), strings.ToUpper(method)+" "+path)
					if pair == nil {
						continue
					}
					for _, name := range []string{"config", "backupConfig"} {
						defs := object(object(pair.properties[name])["$defs"])
						variants := object(defs["keyVariant"])["oneOf"].([]any)
						object(variants[0])["maxLength"] = json.Number("1")
					}
				}
			}
			assertionHash, _ := contractDigest(root)
			if assertionHash == current || len(keyboardIdentityScopes(root)) != 2 {
				t.Fatal("new value assertion was hidden or reviewed scope stopped qualifying")
			}
			root["components"] = map[string]any{"schemas": map[string]any{"Unknown": map[string]any{"$ref": "#/missing"}}}
			unknownBefore, _ := contractDigest(root)
			schema["examples"] = []any{"changed again"}
			unknownAfter, _ := contractDigest(root)
			if unknownBefore == unknownAfter {
				t.Fatal("an unrelated unresolved reference no longer preserves its resource")
			}
		})
	}
}
