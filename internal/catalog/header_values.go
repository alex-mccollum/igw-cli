package catalog

import (
	"mime"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Validate the complete effective fields, then remove their declarations from
// a private view. Header.Get loses empty/repeated values, and the upstream
// parameter decoder skips content and several primitive/array assertions.
func (c *Catalog) headerValidationView(item *v3.PathItem, request *http.Request) (*v3.PathItem, *http.Request, []Issue, error) {
	refuse := func(name, rule string) (*v3.PathItem, *http.Request, []Issue, error) {
		return item, request, []Issue{{Kind: "parameter", Rule: rule, Parameter: name}}, nil
	}
	headers, err := gateway.NormalizeHeaders(request.Header)
	if err != nil {
		return refuse("", "invalid_header")
	}
	op := item.GetOperations().GetOrZero(strings.ToLower(request.Method))
	params := make(map[string]*v3.Parameter)
	for _, list := range [][]*v3.Parameter{item.Parameters, op.Parameters} {
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
		var schema *base.Schema
		var rule string
		if p.Content != nil && p.Content.Len() != 0 {
			if p.Schema != nil || p.Content.Len() != 1 {
				return refuse(name, "unsupported_serialization")
			}
			if len(input) != 1 {
				return refuse(name, "duplicate_parameter")
			}
			for media, content := range p.Content.FromOldest() {
				if content == nil || content.Schema == nil || content.Schema.Schema() == nil {
					return refuse(name, "unsupported_serialization")
				}
				schema = content.Schema.Schema()
				value, rule = decodeHeaderContent(media, input[0])
			}
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
						member, rule := parameterPrimitive(querySchemaKind(schema.Items.A.Schema()), text)
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
	without := func(list []*v3.Parameter) []*v3.Parameter {
		out := make([]*v3.Parameter, 0, len(list))
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

func decodeHeaderContent(media, text string) (any, string) {
	media, parameters, err := mime.ParseMediaType(media)
	if err != nil || strings.Contains(media, "*") {
		return nil, "unsupported_serialization"
	}
	charset := strings.ToLower(parameters["charset"])
	if charset != "" && charset != "utf-8" && charset != "us-ascii" {
		return nil, "unsupported_serialization"
	}
	if charset == "us-ascii" {
		for i := range len(text) {
			if text[i] >= utf8.RuneSelf {
				return nil, "invalid_text"
			}
		}
	}
	_, subtype, _ := strings.Cut(media, "/")
	if subtype == "json" || strings.HasSuffix(subtype, "+json") {
		return decodeExactJSON([]byte(text))
	}
	if media == "text/plain" {
		return parameterPrimitive("string", text)
	}
	return nil, "unsupported_serialization"
}
