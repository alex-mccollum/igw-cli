package catalog

import (
	"net/http"
	"sort"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
)

// Validate complete effective headers, preserving empty and repeated values.
// Normalize names before body media selection; authentication is presence-only.
func (c *Catalog) headerValidationView(item *requestContract, request *http.Request) (*requestContract, *http.Request, []Issue, error) {
	refuse := func(name, rule string) (*requestContract, *http.Request, []Issue, error) {
		return item, request, []Issue{{Kind: "parameter", Rule: rule, Parameter: name}}, nil
	}
	headers, err := gateway.NormalizeHeaders(request.Header)
	if err != nil {
		return refuse("", "invalid_header")
	}
	op := item.Operation
	params := make(map[string]*parameter)
	for _, list := range [][]*parameter{item.Parameters, op.Parameters} {
		seen := make(map[string]bool)
		for _, p := range list {
			if p.In != "header" {
				continue
			}
			name := http.CanonicalHeaderKey(p.Name)
			// OpenAPI explicitly ignores these parameter declarations. Body
			// media selection and Gateway authentication have their own checks.
			if name == "Accept" || name == "Content-Type" || name == "Authorization" {
				continue
			}
			if _, err := gateway.NormalizeHeaders(http.Header{name: nil}); err != nil {
				return refuse(p.Name, "unsupported_serialization")
			}
			if seen[name] {
				return refuse(name, "ambiguous_parameter_binding")
			}
			seen[name], params[name] = true, p
		}
	}
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p, input := params[name], headers[name]
		if len(input) == 0 {
			// These can appear on the wire without an explicit Header entry.
			// This credential/target-free request cannot prove their values.
			switch name {
			case "Host", "Content-Length", "Transfer-Encoding", "User-Agent", "Accept-Encoding":
				return refuse(name, "unsupported_serialization")
			}
			if p.Required != nil && *p.Required {
				return refuse(name, "required")
			}
			continue
		}
		// The execution core supplies only a presence marker here, never a
		// credential. Do not validate the marker as if it were the wire token.
		if name == "X-Ignition-Api-Token" {
			continue
		}
		var bytes int
		for _, text := range input {
			if len(text) > MaxJSONBodyBytes-bytes {
				return refuse(name, "parameter_limit")
			}
			bytes += len(text)
		}
		var value any
		var schema *schemaView
		var rule string
		if p.Content != nil && len(p.Content) != 0 {
			if len(input) != 1 {
				return refuse(name, "duplicate_parameter")
			}
			value, schema, rule = parameterContentValue(p, input[0])
		} else {
			if p.Schema == nil || p.Schema.Schema() == nil || (p.Style != "" && p.Style != "simple") {
				return refuse(name, "unsupported_serialization")
			}
			schema = p.Schema.Schema()
			kind := querySchemaKind(schema)
			if kind == "array" {
				// Simple arrays use commas with either explode value. Repeated
				// HTTP fields contribute to the same ordered list. Only HTTP OWS
				// is stripped; this is neither URI decoding nor an RFC 8941 parser.
				members := make([]any, 0, len(input))
				for _, line := range input {
					for text := range strings.SplitSeq(line, ",") {
						text = strings.Trim(text, " \t")
						// Empty items and an empty list share an ambiguous wire
						// representation. JSON content can express either exactly.
						if text == "" {
							return refuse(name, "ambiguous_parameter_binding")
						}
						member, rule := parameterPrimitive(querySchemaKind(schema.Items.Schema()), text)
						if rule != "" {
							return refuse(name, rule)
						}
						members = append(members, member)
					}
				}
				value = members
			} else {
				if len(input) != 1 {
					return refuse(name, "duplicate_parameter")
				}
				value, rule = parameterPrimitive(kind, input[0])
			}
		}
		if rule != "" {
			return refuse(name, rule)
		}
		issues, err := c.validateParameterValue("header", name, schema, value)
		if err != nil || len(issues) != 0 {
			return item, request, issues, err
		}
	}
	without := func(list []*parameter) []*parameter {
		out := make([]*parameter, 0, len(list))
		for _, p := range list {
			if p.In != "header" {
				out = append(out, p)
			}
		}
		return out
	}
	view, operation := operationValidationView(item, request.Method)
	view.Parameters, operation.Parameters = without(item.Parameters), without(op.Parameters)
	req := *request
	req.Header = headers
	return view, &req, nil, nil
}
