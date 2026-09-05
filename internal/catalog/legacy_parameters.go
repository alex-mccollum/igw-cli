package catalog

import "strconv"

// These defects were observed in the exact 8.3.0 capture. 8.3.9 already declares
// the selected path forms required and provides the cancellation ID's schema.
// The matcher is intentionally limited to those routes and parameter shapes.
func normalizeLegacyParameters(op map[string]any, key, pointer string) []Adjustment {
	params, _ := op["parameters"].([]any)
	var adjustments []Adjustment
	for index, raw := range params {
		param, _ := raw.(map[string]any)
		if !legacyParameterShape(param) {
			continue
		}
		name, _ := param["name"].(string)
		base := pointer + "/parameters/" + strconv.Itoa(index)
		if param["required"] == false && legacyOptionalPath(key, name) {
			schema, _ := param["schema"].(map[string]any)
			if schema["type"] == "string" {
				// A selected path template requires a value at every placeholder.
				// No alternate path is inferred, and all value constraints remain.
				param["required"] = true
				adjustments = append(adjustments, Adjustment{Operation: key, Pointer: base + "/required", Rule: "selected-path-required"})
			}
		}
		_, hasSchema := param["schema"]
		if key == "DELETE /data/api/v1/scripts/cancel-script/{id}" && name == "id" && param["required"] == true && param["description"] == "n/a" && !hasSchema {
			// Empty schema is solely a model placeholder. Validate rejects this
			// operation before reaching the validator; never infer string/UUID
			// assertions from a newer Gateway or from the parameter's name.
			param["schema"] = map[string]any{}
			adjustments = append(adjustments, Adjustment{Operation: key, Pointer: base + "/schema", Rule: "script-cancel-undocumented-id"})
		}
	}
	return adjustments
}

func legacyParameterShape(param map[string]any) bool {
	if param["in"] != "path" || param["style"] != "simple" || param["explode"] != false || param["deprecated"] != false {
		return false
	}
	for key := range param {
		switch key {
		case "name", "in", "description", "required", "deprecated", "style", "explode", "schema":
		default:
			return false
		}
	}
	return true
}

func legacyOptionalPath(key, name string) bool {
	if key == "GET /data/api/v1/entity/section/{section}" {
		return name == "section"
	}
	if name != "scim-version" {
		return false
	}
	switch key {
	case "GET /data/api/v1/scim/{profile-name}/{scim-version}/Groups",
		"POST /data/api/v1/scim/{profile-name}/{scim-version}/Groups",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/Groups/{group-id}",
		"PUT /data/api/v1/scim/{profile-name}/{scim-version}/Groups/{group-id}",
		"DELETE /data/api/v1/scim/{profile-name}/{scim-version}/Groups/{group-id}",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/ResourceTypes",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/ResourceTypes/{type-id}",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/Schemas",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/Schemas/{schema-id}",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/ServiceProviderConfig",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/Users",
		"POST /data/api/v1/scim/{profile-name}/{scim-version}/Users",
		"GET /data/api/v1/scim/{profile-name}/{scim-version}/Users/{user-id}",
		"PUT /data/api/v1/scim/{profile-name}/{scim-version}/Users/{user-id}",
		"DELETE /data/api/v1/scim/{profile-name}/{scim-version}/Users/{user-id}":
		return true
	}
	return false
}
