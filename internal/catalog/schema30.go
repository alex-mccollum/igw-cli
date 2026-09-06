package catalog

import "strings"

// Adapt only schema positions to JSON Schema 2020-12 after OpenAPI 3.0
// admission. Instance data and the exported vendor bytes remain unchanged.
func normalizeSchema30(value any, kind string) {
	if kind == "opaque" {
		return
	}
	if strings.HasPrefix(kind, "map:") {
		if values, ok := value.(map[string]any); ok {
			for _, child := range values {
				normalizeSchema30(child, strings.TrimPrefix(kind, "map:"))
			}
		}
		return
	}
	if strings.HasPrefix(kind, "array:") {
		if values, ok := value.([]any); ok {
			for _, child := range values {
				normalizeSchema30(child, strings.TrimPrefix(kind, "array:"))
			}
		}
		return
	}
	values, ok := value.(map[string]any)
	if !ok {
		return
	}
	for name, child := range values {
		normalizeSchema30(child, projectionChild(kind, name, child))
	}
	if kind != "schema" {
		return
	}
	if nullable, _ := values["nullable"].(bool); nullable {
		// OAS 3.0 adds null only to a type declared in the same schema.
		// Enum, composition, and other assertions continue to apply.
		if declared, ok := values["type"].(string); ok {
			values["type"] = []any{declared, "null"}
		}
	}
	delete(values, "nullable")
	for _, bound := range []string{"minimum", "maximum"} {
		exclusive := "exclusive" + strings.ToUpper(bound[:1]) + bound[1:]
		if enabled, ok := values[exclusive].(bool); ok {
			delete(values, exclusive)
			if number, present := values[bound]; enabled && present {
				delete(values, bound)
				values[exclusive] = number
			}
		}
	}
}
