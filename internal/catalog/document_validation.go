package catalog

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Validate the exact decoded document against its pinned OpenAPI metaschema.
// Reference existence and selected request constraints have separate checks.
func validDocumentValue(value any, version string) bool {
	if !documentNumbersSupported(value) {
		return false
	}
	load := documentSchema30
	if strings.HasPrefix(version, "3.1.") {
		load = documentSchema31
	}
	schema, err := load()
	return err == nil && schema.Validate(value) == nil
}

func documentNumbersSupported(value any) bool {
	switch value := value.(type) {
	case json.Number:
		// Preserve the upstream document path's float64-range admission check,
		// while the schema compiler still receives the original exact number.
		// Also bound numeric work, including deeply underflowing exponents that
		// float64 decoding alone silently converts to zero.
		if _, rule := parameterPrimitive("number", string(value)); rule != "" {
			return false
		}
		_, err := strconv.ParseFloat(string(value), 64)
		return err == nil
	case map[string]any:
		for _, child := range value {
			if !documentNumbersSupported(child) {
				return false
			}
		}
	case []any:
		for _, child := range value {
			if !documentNumbersSupported(child) {
				return false
			}
		}
	}
	return true
}
