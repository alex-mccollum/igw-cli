package catalog

import "strings"

// libopenapi's object-backed Schema positions lose boolean schema semantics.
// Equivalent object forms survive: true is {} and false is {"not":{}}. Visit
// actual schema positions, never false annotations, examples, or instance data.
// This is a parser representation fix for valid OAS 3.1, not an IA correction.
// Original document validation, exports, descriptions, and identities use the
// original bytes; only the private model input is rewritten.
func normalizeBooleanSchemas(value any, kind string) (any, bool) {
	if kind == "opaque" {
		return value, false
	}
	if kind == "schema" {
		if boolean, ok := value.(bool); ok {
			if boolean {
				return map[string]any{}, true
			}
			return map[string]any{"not": map[string]any{}}, true
		}
	}
	changed := false
	if strings.HasPrefix(kind, "map:") {
		if values, ok := value.(map[string]any); ok {
			for name, child := range values {
				adapted, found := normalizeBooleanSchemas(child, strings.TrimPrefix(kind, "map:"))
				values[name], changed = adapted, changed || found
			}
		}
		return value, changed
	}
	if strings.HasPrefix(kind, "array:") {
		if values, ok := value.([]any); ok {
			for i, child := range values {
				adapted, found := normalizeBooleanSchemas(child, strings.TrimPrefix(kind, "array:"))
				values[i], changed = adapted, changed || found
			}
		}
		return value, changed
	}
	if values, ok := value.(map[string]any); ok {
		for name, child := range values {
			// These three fields have explicit boolean variants in the model.
			// Keep their supported representation to avoid rebuilding ordinary
			// Gateway documents solely for additionalProperties: false.
			if kind == "schema" && (name == "items" || name == "additionalProperties" || name == "unevaluatedProperties") {
				if _, boolean := child.(bool); boolean {
					continue
				}
			}
			adapted, found := normalizeBooleanSchemas(child, projectionChild(kind, name, child))
			values[name], changed = adapted, changed || found
		}
	}
	return value, changed
}
