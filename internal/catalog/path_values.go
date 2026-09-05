package catalog

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Bind the selected operation's relative path, never the document's servers.
// The transport owns the explicit Gateway target and its reverse-proxy prefix.
// Validated declarations are removed only from a private view so upstream
// segment indexing, coercion, and inheritance cannot reinterpret these values.
func (c *Catalog) pathValidationView(item *v3.PathItem, request *http.Request, template string) (*v3.PathItem, []Issue, error) {
	refuse := func(name, rule string) (*v3.PathItem, []Issue, error) {
		return item, []Issue{{Kind: "parameter", Rule: rule, Parameter: name}}, nil
	}
	values, rule := bindPathValues(template, request.URL.EscapedPath())
	if rule != "" {
		return refuse("", rule)
	}
	op := item.GetOperations().GetOrZero(strings.ToLower(request.Method))
	params := make(map[string]*v3.Parameter)
	for _, list := range [][]*v3.Parameter{item.Parameters, op.Parameters} {
		seen := make(map[string]bool)
		for _, p := range list {
			if p.In != "path" {
				continue
			}
			if seen[p.Name] {
				return refuse(p.Name, "ambiguous_parameter_binding")
			}
			seen[p.Name], params[p.Name] = true, p
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := params[name]
		if p == nil {
			return refuse(name, "undeclared_path_parameter")
		}
		if values[name] == "" {
			return refuse(name, "required")
		}
		if p.Schema == nil || p.Schema.Schema() == nil || (p.Style != "" && p.Style != "simple") {
			return refuse(name, "unsupported_serialization")
		}
		schema := p.Schema.Schema()
		kind := querySchemaKind(schema)
		if kind == "" || kind == "array" {
			return refuse(name, "unsupported_serialization")
		}
		value, rule := parameterPrimitive(kind, values[name])
		if rule != "" {
			return refuse(name, rule)
		}
		issues, err := c.validateParameterValue("path", name, schema, value)
		if err != nil || len(issues) != 0 {
			return item, issues, err
		}
	}
	if len(params) != len(values) {
		return refuse("", "unbound_path_parameter")
	}
	without := func(list []*v3.Parameter) []*v3.Parameter {
		out := make([]*v3.Parameter, 0, len(list))
		for _, p := range list {
			if p.In != "path" {
				out = append(out, p)
			}
		}
		return out
	}
	view, operation := operationValidationView(item, request.Method)
	view.Parameters, operation.Parameters = without(item.Parameters), without(op.Parameters)
	return view, nil, nil
}

func bindPathValues(template, escapedPath string) (map[string]string, string) {
	parts, submitted := strings.Split(template, "/"), strings.Split(escapedPath, "/")
	if len(parts) != len(submitted) {
		return nil, "path_mismatch"
	}
	values := make(map[string]string)
	for i, part := range parts {
		// Split before decoding: an encoded slash belongs to the parameter,
		// never to the path hierarchy. Percent escapes are decoded once.
		input, err := url.PathUnescape(submitted[i])
		if err != nil {
			return nil, "invalid_path_encoding"
		}
		if !strings.ContainsAny(part, "{}") {
			literal, err := url.PathUnescape(part)
			if err != nil {
				return nil, "unsupported_path_template"
			}
			if input != literal {
				return nil, "path_mismatch"
			}
			continue
		}
		var pattern strings.Builder
		pattern.WriteString("(?s)^")
		var names []string
		for {
			start := strings.IndexByte(part, '{')
			literal := part
			if start >= 0 {
				literal = part[:start]
			}
			if strings.Contains(literal, "}") {
				return nil, "unsupported_path_template"
			}
			literal, err = url.PathUnescape(literal)
			if err != nil {
				return nil, "unsupported_path_template"
			}
			pattern.WriteString(regexp.QuoteMeta(literal))
			if start < 0 {
				break
			}
			part = part[start+1:]
			end := strings.IndexByte(part, '}')
			if end <= 0 || strings.Contains(part[:end], "{") {
				return nil, "unsupported_path_template"
			}
			names = append(names, part[:end])
			pattern.WriteString("(.*?)")
			part = part[end+1:]
		}
		pattern.WriteString("$")
		matcher, err := regexp.Compile(pattern.String())
		if err != nil {
			return nil, "unsupported_path_template"
		}
		matches := matcher.FindStringSubmatch(input)
		if len(matches) != len(names)+1 {
			return nil, "path_mismatch"
		}
		if len(names) > 1 {
			// Refuse an ambiguous division between expressions, rather than
			// claiming checks against a value different from the caller's input.
			greedy, err := regexp.Compile(strings.ReplaceAll(pattern.String(), "(.*?)", "(.*)"))
			if err != nil {
				return nil, "unsupported_path_template"
			}
			other := greedy.FindStringSubmatch(input)
			for j := 1; j < len(matches); j++ {
				if len(other) != len(matches) || matches[j] != other[j] {
					return nil, "ambiguous_parameter_binding"
				}
			}
		}
		for j, name := range names {
			value := matches[j+1]
			if previous, present := values[name]; present && previous != value {
				return nil, "ambiguous_parameter_binding"
			}
			values[name] = value
		}
	}
	return values, ""
}
