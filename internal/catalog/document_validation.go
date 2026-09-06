package catalog

import (
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"

	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/helpers"
	"github.com/pb33f/libopenapi/datamodel"
)

// Validate the exact JSON value with the same embedded document schema and
// compiler used by ValidateOpenAPIDocument. JSON decoding and duplicate-key
// checks have already happened; a second YAML tree is unnecessary for this
// boolean check. BuildV3Model still resolves and validates the actual model.
func validDocumentValue(value any, version string) bool {
	if !documentNumbersSupported(value) {
		return false
	}
	schema := datamodel.OpenAPI3SchemaData
	if strings.HasPrefix(version, "3.1.") {
		schema = datamodel.OpenAPI31SchemaData
	}
	options := validatorconfig.NewValidationOptions(
		validatorconfig.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	compiled, err := helpers.NewCompiledSchema("schema", []byte(schema), options)
	return err == nil && compiled.Validate(value) == nil
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
