package catalog

import "strings"

// Validated 3.0 schemas are adapted to the compiler's 3.1 schema dialect in the
// private model. Keep OpenAPI vocabulary checks enabled after that adaptation.
const validationSchemaVersion float32 = 3.1

// The upstream 3.0 compiler converts schemas with json.Unmarshal into float64
// and recursively rewrites instance data too. Adapt only schema positions in
// our exact-number private model, after validating the original 3.0 document.
// Neither exported vendor evidence nor its declared OpenAPI version changes.
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
				// The 3.0 renderer turns numeric exclusive bounds back into
				// booleans. Keep the inclusive bound and exclude equality using
				// a schema conjunction. Enum applies only to that exact number.
				allOf, _ := values["allOf"].([]any)
				values["allOf"] = append(allOf, map[string]any{"not": map[string]any{"enum": []any{number}}})
			}
		}
	}
}
