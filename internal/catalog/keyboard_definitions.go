package catalog

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// 8.3.0 embeds the keyboard schema without rebasing its local $defs references.
// Its two definitions form a fixed, acyclic graph. Expanding that graph gives
// exactly the inline schema emitted by the captured 8.3.9 Gateway. Only the
// reviewed operations, paired schemas, and reference positions qualify. Every
// supplied assertion survives; original evidence remains outside this model.
func normalizeKeyboardDefinitions(op map[string]any, key, pointer string) []Adjustment {
	var parts []string
	switch key {
	case "POST /data/api/v1/resources/ignition/keyboard_layout", "PUT /data/api/v1/resources/ignition/keyboard_layout":
		parts = []string{"requestBody", "content", "application/json", "schema", "items", "properties"}
	case "GET /data/api/v1/resources/find/ignition/keyboard_layout/{name}":
		parts = []string{"responses", "200", "content", "application/json", "schema", "properties"}
	case "GET /data/api/v1/resources/list/ignition/keyboard_layout":
		parts = []string{"responses", "200", "content", "application/json", "schema", "properties", "items", "items", "properties"}
	default:
		return nil
	}
	properties := op
	base := ""
	for _, part := range parts {
		properties = object(properties[part])
		base += "/" + pointerEscape(part)
	}
	primary, backup := object(properties["config"]), object(properties["backupConfig"])
	a, _ := json.Marshal(primary)
	b, _ := json.Marshal(backup)
	// Besides limiting expansion, identical pairs prevent a partial correction
	// when one side of an unfamiliar generator shape has changed.
	if primary == nil || len(a) > 64<<10 || !bytes.Equal(a, b) || primary["type"] != "object" {
		return nil
	}
	defs := object(primary["$defs"])
	if len(defs) != 2 || object(defs["key"]) == nil || object(defs["keyVariant"]) == nil {
		return nil
	}
	allowed := make(map[string]string)
	for _, name := range []string{"config", "backupConfig"} {
		root := base + "/" + name
		allowed[root+"/properties/rows/items/items"] = "#/$defs/key"
		allowed[root+"/$defs/key/oneOf/0/properties/lowercase"] = "#/$defs/keyVariant"
		allowed[root+"/$defs/key/oneOf/0/properties/uppercase"] = "#/$defs/keyVariant"
	}
	seen := make(map[string]bool)
	if !keyboardReferenceScope(op, "", base, allowed, seen) || len(seen) != len(allowed) {
		return nil
	}
	var adjustments []Adjustment
	for _, name := range []string{"config", "backupConfig"} {
		properties[name] = expandKeyboardDefinitions(object(properties[name]), defs)
		adjustments = append(adjustments, Adjustment{Operation: key, Pointer: pointer + base + "/" + name, Rule: "keyboard-local-definitions"})
	}
	return adjustments
}

// Scan the entire operation before changing anything. Any unknown reference,
// identity, anchor, dialect, or definition scope prevents this correction.
// Conservatively checking instance examples as well cannot weaken a contract.
func keyboardReferenceScope(value any, path, base string, allowed map[string]string, seen map[string]bool) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch {
			case key == "$ref":
				if len(v) != 1 || allowed[path] == "" || child != allowed[path] {
					return false
				}
				seen[path] = true
			case key == "$defs":
				if path != base+"/config" && path != base+"/backupConfig" {
					return false
				}
			case strings.HasPrefix(key, "$") || key == "discriminator" || key == "jsonSchemaDialect":
				return false
			}
			if !keyboardReferenceScope(child, path+"/"+pointerEscape(key), base, allowed, seen) {
				return false
			}
		}
	case []any:
		for index, child := range v {
			if !keyboardReferenceScope(child, path+"/"+strconv.Itoa(index), base, allowed, seen) {
				return false
			}
		}
	}
	return true
}

// Called only after the fixed reference graph is verified above. All reference
// objects contain solely $ref; no sibling assertions need merging. $defs does
// not assert anything about instances and is removed after its uses expand.
func expandKeyboardDefinitions(value any, defs map[string]any) any {
	switch v := value.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			return expandKeyboardDefinitions(defs[strings.TrimPrefix(ref, "#/$defs/")], defs)
		}
		out := make(map[string]any, len(v))
		for key, child := range v {
			if key != "$defs" {
				out[key] = expandKeyboardDefinitions(child, defs)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for index, child := range v {
			out[index] = expandKeyboardDefinitions(child, defs)
		}
		return out
	}
	return value
}
