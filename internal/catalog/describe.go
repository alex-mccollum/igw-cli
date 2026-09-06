package catalog

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
)

// compactDescription retains original vendor definitions, rather than emitting
// the adjusted validation tree. Whole named components keep local $defs intact.
// References with document-wide scope fall back to the complete original
// document; discovery must not present a misleading partial definition.
func (c *Catalog) compactDescription(d *Description) error {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(d.PathItem, &item); err != nil {
		return err
	}
	fallback := false
	if _, ok := item["$ref"]; ok {
		fallback = true
	}
	for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
		if method != strings.ToLower(d.Operation.Method) {
			delete(item, method)
		}
	}
	item[strings.ToLower(d.Operation.Method)] = d.Operation.Definition
	var operation map[string]json.RawMessage
	if err := json.Unmarshal(d.Operation.Definition, &operation); err != nil {
		return err
	}
	if security, ok := operation["security"]; ok {
		d.Security = security
	}
	var components map[string]map[string]json.RawMessage
	if len(c.root["components"]) > 0 {
		if err := json.Unmarshal(c.root["components"], &components); err != nil {
			return err
		}
	}
	selected := map[string]map[string]json.RawMessage{}
	var queue []json.RawMessage
	add := func(category, name string) {
		if selected[category] != nil && selected[category][name] != nil {
			return
		}
		raw := components[category][name]
		if raw == nil {
			fallback = true
			return
		}
		if selected[category] == nil {
			selected[category] = map[string]json.RawMessage{}
		}
		selected[category][name] = raw
		queue = append(queue, raw)
	}
	reference := func(ref string) {
		decoded, err := url.PathUnescape(ref)
		if err != nil || !strings.HasPrefix(decoded, "#/components/") {
			fallback = true
			return
		}
		parts := strings.Split(strings.TrimPrefix(decoded, "#/components/"), "/")
		if len(parts) < 2 {
			fallback = true
			return
		}
		unescape := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~") }
		add(unescape(parts[0]), unescape(parts[1]))
	}
	var security []map[string]json.RawMessage
	if len(d.Security) > 0 {
		if err := json.Unmarshal(d.Security, &security); err != nil {
			return err
		}
		for _, requirement := range security {
			for name := range requirement {
				add("securitySchemes", name)
			}
		}
	}
	var walk func(any, bool)
	walk = func(value any, scoped bool) {
		switch v := value.(type) {
		case map[string]any:
			if _, ok := v["$id"]; ok {
				scoped = true
			}
			for key, child := range v {
				if ref, ok := child.(string); ok {
					switch key {
					case "$dynamicRef", "$recursiveRef", "operationRef":
						fallback = true
					case "$ref":
						// Local references inside a retained resource are already included.
						// Anchors or cross-resource references need the document-wide context.
						if scoped && strings.HasPrefix(ref, "#/$defs/") {
							break
						}
						reference(ref)
					}
				}
				if key == "discriminator" {
					if disc, ok := child.(map[string]any); ok {
						if mapping, ok := disc["mapping"].(map[string]any); ok {
							for _, mapped := range mapping {
								if name, ok := mapped.(string); ok {
									if strings.ContainsAny(name, "/#:") {
										reference(name)
									} else {
										add("schemas", name)
									}
								}
							}
						}
					}
				}
				walk(child, scoped)
			}
		case []any:
			for _, child := range v {
				walk(child, scoped)
			}
		}
	}
	for _, raw := range item {
		queue = append(queue, raw)
	}
	for len(queue) > 0 && !fallback {
		raw := queue[0]
		queue = queue[1:]
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		err := decoder.Decode(&value)
		if err != nil {
			return err
		}
		walk(value, false)
	}
	var err error
	d.PathItem, err = json.Marshal(item)
	if err != nil {
		return err
	}
	if fallback {
		d.Document = bytes.Clone(c.raw)
		d.Gaps = append(d.Gaps, "Reference scope requires the original document; document contains the complete context.")
	} else if len(selected) > 0 {
		d.Components, err = json.Marshal(selected)
	}
	return err
}
