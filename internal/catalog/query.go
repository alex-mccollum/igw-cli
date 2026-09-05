package catalog

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/schema_validation"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Exploded form objects use their property names as query keys. The upstream
// validator instead binds all query keys to an absent object, strips brackets
// from property names, and stops checking subsequent parameters. Bind the
// advertised filter separately, validate its complete schema, then validate the
// remaining request through a private view. Neither vendor data nor wire input
// is changed. Unsupported or overlapping ownership is refused explicitly.
func (c *Catalog) filterValidationView(item *v3.PathItem, request *http.Request) (*v3.PathItem, *http.Request, []Issue, error) {
	if item == nil {
		return item, request, nil, nil
	}
	operation := item.GetOperations().GetOrZero(strings.ToLower(request.Method))
	if operation == nil {
		return item, request, nil, nil
	}
	// Operation declarations override inherited declarations by name/location.
	queries := make(map[string]*v3.Parameter)
	duplicate := false
	for _, list := range [][]*v3.Parameter{item.Parameters, operation.Parameters} {
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
	refuse := func(rule string) (*v3.PathItem, *http.Request, []Issue, error) {
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
		if peer.Schema == nil || peer.Schema.Schema() == nil || matcher.MatchString(name) {
			return refuse("ambiguous_parameter_binding")
		}
		// Only named scalar peers have qualified, disjoint wire ownership here.
		types := peer.Schema.Schema().Type
		if len(types) != 1 || (types[0] != "string" && types[0] != "integer" && types[0] != "number" && types[0] != "boolean") || (peer.Style != "" && peer.Style != "form") {
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
		v := schema_validation.NewSchemaValidatorWithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)), validatorconfig.WithSchemaCache(nil))
		defer v.Release()
		version := float32(3.1)
		if strings.HasPrefix(c.model.Model.Version, "3.0.") {
			version = 3.0
		}
		valid, failures := v.ValidateSchemaObjectWithVersion(schema, properties, version)
		if !valid {
			issues, err := validationIssues(failures)
			if err != nil || len(issues) == 0 {
				return item, request, nil, ErrSchemaCompilation
			}
			for i := range issues {
				issues[i].Kind, issues[i].Rule, issues[i].Parameter = "parameter", "query", "filter"
			}
			return item, request, issues, nil
		}
	}
	withoutFilter := func(params []*v3.Parameter) []*v3.Parameter {
		out := make([]*v3.Parameter, 0, len(params))
		for _, p := range params {
			if p.In != "query" || p.Name != "filter" {
				out = append(out, p)
			}
		}
		return out
	}
	view, op := *item, *operation
	view.Parameters = withoutFilter(item.Parameters)
	op.Parameters = withoutFilter(operation.Parameters)
	switch request.Method {
	case http.MethodGet:
		view.Get = &op
	case http.MethodPost:
		view.Post = &op
	case http.MethodPut:
		view.Put = &op
	case http.MethodPatch:
		view.Patch = &op
	case http.MethodDelete:
		view.Delete = &op
	case http.MethodHead:
		view.Head = &op
	case http.MethodOptions:
		view.Options = &op
	case http.MethodTrace:
		view.Trace = &op
	}
	// Remove filter properties from the private request too. Otherwise a key
	// such as name[eq] can be misread as a value for a separate name parameter.
	req, u := *request, *request.URL
	u.RawQuery = values.Encode()
	req.URL = &u
	return &view, &req, nil, nil
}
