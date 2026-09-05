package catalog

import (
	"net/http"
	"regexp"
	"strings"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// libopenapi-validator 0.14.0 binds every query key to a missing exploded
// object with additionalProperties. Ignition's optional list filter then
// wrongly receives limit/offset/search. Omit only a provably absent filter
// from a private validation view: all supplied keys must identify other scalar
// parameters and violate the filter's property-name pattern. Supplied or
// ambiguous filter inputs fail closed until their serialization is qualified.
func listValidationView(item *v3.PathItem, request *http.Request, path string) (*v3.PathItem, *Issue) {
	qualifiedList := strings.HasPrefix(path, "/data/api/v1/resources/list/") || path == "/data/api/v1/projects/list" || path == "/data/api/v1/logs"
	if item == nil || item.Get == nil || request.Method != "GET" || !qualifiedList || len(item.Parameters) != 0 {
		return item, nil
	}
	var filter *v3.Parameter
	scalars := map[string]bool{}
	for _, parameter := range item.Get.Parameters {
		if parameter.In != "query" || parameter.Schema == nil {
			continue
		}
		schema := parameter.Schema.Schema()
		if schema == nil || len(schema.Type) != 1 {
			return item, nil
		}
		if parameter.Name == "filter" {
			filter = parameter
		} else if schema.Type[0] == "string" || schema.Type[0] == "integer" || schema.Type[0] == "number" || schema.Type[0] == "boolean" {
			scalars[parameter.Name] = true
		}
	}
	if filter == nil || filter.Required != nil && *filter.Required || !filter.IsDefaultFormEncoding() {
		return item, nil
	}
	schema := filter.Schema.Schema()
	if schema.Type[0] != "object" || schema.PropertyNames == nil || schema.PropertyNames.Schema() == nil {
		return item, nil
	}
	pattern := schema.PropertyNames.Schema().Pattern
	if pattern == "" {
		return item, nil
	}
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		return item, nil
	}
	for key := range request.URL.Query() {
		if !scalars[key] || matcher.MatchString(key) {
			return item, &Issue{Kind: "parameter", Rule: "unsupported_serialization", Parameter: "filter"}
		}
	}
	copy := *item
	operation := *item.Get
	operation.Parameters = make([]*v3.Parameter, 0, len(item.Get.Parameters)-1)
	for _, parameter := range item.Get.Parameters {
		if parameter != filter {
			operation.Parameters = append(operation.Parameters, parameter)
		}
	}
	copy.Get = &operation
	return &copy, nil
}
