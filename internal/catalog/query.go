package catalog

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
)

// Exploded filter objects own their property names as query keys. Bind and
// validate the complete object before the remaining named parameters; refuse
// overlapping or unsupported ownership without changing wire values.
func (c *Catalog) filterValidationView(item *requestContract, request *http.Request) (*requestContract, *http.Request, []Issue, error) {
	if item == nil {
		return item, request, nil, nil
	}
	operation := item.Operation
	if operation == nil {
		return item, request, nil, nil
	}
	// Operation declarations override inherited declarations by name/location.
	queries := make(map[string]*parameter)
	duplicate := false
	for _, list := range [][]*parameter{item.Parameters, operation.Parameters} {
		seen := make(map[string]bool)
		for _, p := range list {
			if p.In == "query" {
				duplicate = duplicate || seen[p.Name]
				seen[p.Name] = true
				queries[p.Name] = p
			}
		}
	}
	filter := queries["filter"]
	if filter == nil || filter.Schema == nil {
		return item, request, nil, nil
	}
	schema := filter.Schema.Schema()
	if schema == nil || len(schema.Type) != 1 || schema.Type[0] != "object" {
		return item, request, nil, nil
	}
	refuse := func(rule string) (*requestContract, *http.Request, []Issue, error) {
		return item, request, []Issue{{Kind: "parameter", Rule: rule, Parameter: "filter"}}, nil
	}
	if duplicate {
		return refuse("ambiguous_parameter_binding")
	}
	if !filter.IsDefaultFormEncoding() || schema.PropertyNames == nil || schema.PropertyNames.Schema() == nil || schema.PropertyNames.Schema().Pattern == "" {
		return refuse("unsupported_serialization")
	}
	matcher, err := regexp.Compile(schema.PropertyNames.Schema().Pattern)
	if err != nil {
		return refuse("unsupported_serialization")
	}
	for name, peer := range queries {
		if name == "filter" {
			continue
		}
		if matcher.MatchString(name) {
			return refuse("ambiguous_parameter_binding")
		}
		// Content parameters own one exact query key. The named-parameter
		// decoder checks their representation later, independently of filters.
		if peer.Content != nil && len(peer.Content) != 0 {
			if peer.Schema != nil || len(peer.Content) != 1 {
				return refuse("unsupported_serialization")
			}
			continue
		}
		if peer.Schema == nil || peer.Schema.Schema() == nil {
			return refuse("ambiguous_parameter_binding")
		}
		// Named primitive/array peers own only their exact query key.
		kind := querySchemaKind(peer.Schema.Schema())
		if kind == "" || (peer.Style != "" && peer.Style != "form") || (kind == "array" && !peer.IsDefaultFormEncoding()) {
			return refuse("unsupported_serialization")
		}
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return refuse("invalid_query_encoding")
	}
	properties := make(map[string]any)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "filter" {
			return refuse("unsupported_serialization")
		}
		if _, named := queries[key]; named {
			continue
		}
		if len(values[key]) != 1 {
			return refuse("duplicate_filter_property")
		}
		// IA's filter property schema permits strings as well as numbers and
		// booleans. Preserve the exact decoded text, including numeric-looking
		// strings, percent characters, brackets, plus signs, and empty values.
		// Schema assertions below decide which string values are valid.
		properties[key] = values[key][0]
		delete(values, key)
	}
	if len(properties) == 0 {
		if filter.Required != nil && *filter.Required {
			return refuse("required")
		}
	} else {
		issues, err := c.validateParameterValue("query", "filter", schema, properties)
		if err != nil || len(issues) > 0 {
			return item, request, issues, err
		}
	}
	// Bound filter properties must be absent from the private request too:
	// name[eq] cannot supply a distinct named parameter called name.
	view, req := queryValidationView(item, request, map[string]bool{"filter": true}, values)
	return view, req, nil, nil
}
