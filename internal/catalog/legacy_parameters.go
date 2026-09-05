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
		if kind := legacyPathType(key, name); kind != "" && param["required"] == false {
			schema, _ := param["schema"].(map[string]any)
			if schema["type"] == kind {
				// A selected path template requires a value at every placeholder.
				// No alternate path is inferred, and all value constraints remain.
				param["required"] = true
				adjustments = append(adjustments, Adjustment{Operation: key, Pointer: base + "/required", Rule: "selected-path-required"})
			}
		}
		_, hasSchema := param["schema"]
		if rule := legacyUndocumentedPath(key, name); rule != "" && param["required"] == true && param["description"] == "n/a" && !hasSchema {
			// Empty schema is solely a model placeholder. Validate rejects this
			// operation before reaching the validator; never infer string/UUID
			// assertions from a newer Gateway or from the parameter's name.
			param["schema"] = map[string]any{}
			adjustments = append(adjustments, Adjustment{Operation: key, Pointer: base + "/schema", Rule: rule})
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

func legacyUndocumentedPath(key, name string) string {
	if key == "DELETE /data/api/v1/scripts/cancel-script/{id}" && name == "id" {
		return "script-cancel-undocumented-id"
	}
	if key == "GET /data/sfc/api/v1/charts/{projectName}/{chartPath}" && (name == "projectName" || name == "chartPath") {
		return "sfc-undocumented-path"
	}
	return ""
}

func legacyPathType(key, name string) string {
	if key == "GET /data/eam/api/v1/eam-tasks/scheduled/{running}" && name == "running" {
		return "boolean"
	}
	if key == "GET /data/api/v1/entity/section/{section}" && name == "section" {
		return "string"
	}
	if name != "scim-version" {
		return ""
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
		return "string"
	}
	return ""
}
