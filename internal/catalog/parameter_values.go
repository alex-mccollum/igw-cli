package catalog

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/schema_validation"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Bound exact numeric validation before the schema library constructs rational
// numbers. Wire text remains unchanged; these are validation work limits.
const maxParameterNumberChars = 4096
const maxParameterNumberExponent = 4096

var integerText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
var numberText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// namedQueryValidationView validates complete typed values, including array
// cardinality/uniqueness and primitive constraints. The upstream validator
// splits each repeated array value again and omits some primitive assertions.
// All supported query declarations are removed from a private validation view
// so that header/path/body validation cannot repeat that lossy interpretation.
func (c *Catalog) namedQueryValidationView(item *v3.PathItem, request *http.Request) (*v3.PathItem, *http.Request, []Issue, error) {
	if item == nil {
		return item, request, nil, nil
	}
	op := item.GetOperations().GetOrZero(strings.ToLower(request.Method))
	if op == nil {
		return item, request, nil, nil
	}
	refuse := func(name, rule string) (*v3.PathItem, *http.Request, []Issue, error) {
		return item, request, []Issue{{Kind: "parameter", Rule: rule, Parameter: name}}, nil
	}
	queries := make(map[string]*v3.Parameter)
	for _, list := range [][]*v3.Parameter{item.Parameters, op.Parameters} {
		seen := make(map[string]bool)
		for _, p := range list {
			if p.In != "query" {
				continue
			}
			if seen[p.Name] {
				return refuse(p.Name, "ambiguous_parameter_binding")
			}
			seen[p.Name], queries[p.Name] = true, p
		}
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return refuse("", "invalid_query_encoding")
	}
	names := make([]string, 0, len(queries))
	for name := range queries {
		names = append(names, name)
	}
	sort.Strings(names)
	removed := make(map[string]bool, len(names))
	for _, name := range names {
		p := queries[name]
		if p.Schema == nil || p.Schema.Schema() == nil {
			return refuse(name, "unsupported_serialization")
		}
		schema := p.Schema.Schema()
		kind := querySchemaKind(schema)
		if kind == "" || (p.Style != "" && p.Style != "form") || (kind == "array" && !p.IsDefaultFormEncoding()) {
			return refuse(name, "unsupported_serialization")
		}
		input, present := values[name]
		if !present {
			if p.Required != nil && *p.Required {
				return refuse(name, "required")
			}
		} else {
			var value any
			if kind == "array" {
				members := make([]any, 0, len(input))
				for _, text := range input {
					member, rule := parameterPrimitive(querySchemaKind(schema.Items.A.Schema()), text)
					if rule != "" {
						return refuse(name, rule)
					}
					members = append(members, member)
				}
				value = members
			} else {
				if len(input) != 1 {
					return refuse(name, "duplicate_parameter")
				}
				var rule string
				value, rule = parameterPrimitive(kind, input[0])
				if rule != "" {
					return refuse(name, rule)
				}
			}
			issues, err := c.validateParameterValue("query", name, schema, value)
			if err != nil || len(issues) > 0 {
				return item, request, issues, err
			}
		}
		removed[name] = true
		delete(values, name)
	}
	view, req := queryValidationView(item, request, removed, values)
	return view, req, nil, nil
}

func querySchemaKind(schema *base.Schema) string {
	if schema == nil {
		return ""
	}
	kinds := schema.Type
	// Null has no implicit parameter spelling. A nullable schema still checks
	// the supplied primitive; the caller never substitutes null for text.
	if len(kinds) == 2 {
		if kinds[0] == "null" {
			kinds = kinds[1:]
		} else if kinds[1] == "null" {
			kinds = kinds[:1]
		}
	}
	if len(kinds) != 1 {
		return ""
	}
	switch kind := kinds[0]; kind {
	case "string", "integer", "number", "boolean":
		return kind
	case "array":
		if schema.Items != nil && schema.Items.IsA() && schema.Items.A != nil {
			kind := querySchemaKind(schema.Items.A.Schema())
			if kind != "" && kind != "array" {
				return "array"
			}
		}
	}
	return ""
}

func parameterPrimitive(kind, text string) (any, string) {
	switch kind {
	case "string":
		if !utf8.ValidString(text) {
			return nil, "invalid_utf8"
		}
		return text, ""
	case "boolean":
		if text != "true" && text != "false" {
			return nil, "type"
		}
		return text == "true", ""
	case "integer", "number":
		if len(text) > maxParameterNumberChars {
			return nil, "numeric_limit"
		}
		pattern := numberText
		if kind == "integer" {
			pattern = integerText
		}
		if !pattern.MatchString(text) {
			return nil, "type"
		}
		if at := strings.IndexAny(text, "eE"); at >= 0 {
			exponent, err := strconv.Atoi(text[at+1:])
			if err != nil || exponent < -maxParameterNumberExponent || exponent > maxParameterNumberExponent {
				return nil, "numeric_limit"
			}
		}
		return json.Number(text), ""
	}
	return nil, "unsupported_serialization"
}

// Caller holds Catalog.validationMu: rendering can mutate upstream schema state.
func (c *Catalog) validateParameterValue(location, name string, schema *base.Schema, value any) ([]Issue, error) {
	v := schema_validation.NewSchemaValidatorWithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)), validatorconfig.WithSchemaCache(nil))
	defer v.Release()
	valid, failures := v.ValidateSchemaObjectWithVersion(schema, value, validationSchemaVersion)
	if valid {
		return nil, nil
	}
	issues, err := validationIssues(failures)
	if err != nil || len(issues) == 0 {
		return nil, ErrSchemaCompilation
	}
	for i := range issues {
		issues[i].Kind, issues[i].Rule, issues[i].Parameter = "parameter", location, name
	}
	return issues, nil
}

func queryValidationView(item *v3.PathItem, request *http.Request, removed map[string]bool, values url.Values) (*v3.PathItem, *http.Request) {
	without := func(params []*v3.Parameter) []*v3.Parameter {
		out := make([]*v3.Parameter, 0, len(params))
		for _, p := range params {
			if p.In != "query" || !removed[p.Name] {
				out = append(out, p)
			}
		}
		return out
	}
	view, op := operationValidationView(item, request.Method)
	view.Parameters, op.Parameters = without(item.Parameters), without(op.Parameters)
	req, u := *request, *request.URL
	u.RawQuery = values.Encode()
	req.URL = &u
	return view, &req
}

// Callers have already resolved this operation. Copy only the selected path
// and operation; shared schemas stay protected by Catalog.validationMu.
func operationValidationView(item *v3.PathItem, method string) (*v3.PathItem, *v3.Operation) {
	view, op := *item, *item.GetOperations().GetOrZero(strings.ToLower(method))
	switch method {
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
	return &view, &op
}
