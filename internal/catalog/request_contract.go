package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
)

// These views contain only wire-binding information for the selected operation.
// Schemas remain pointers into the JSON document, including recursive schemas.
type requestContract struct {
	Parameters []*parameter
	Operation  *operationContract
}

func (c *Catalog) expandPathItems() error {
	paths, _ := c.value["paths"].(map[string]any)
	for path, value := range paths {
		item, _ := value.(map[string]any)
		seen := make(map[string]bool)
		for item["$ref"] != nil {
			ref, ok := item["$ref"].(string)
			if !ok || !strings.HasPrefix(ref, "#/") || seen[ref] {
				return errors.New("invalid path item reference")
			}
			seen[ref] = true
			pointer, err := url.PathUnescape(ref[1:])
			if err != nil {
				return err
			}
			base, err := pointerValue(c.value, pointer)
			if err != nil {
				return err
			}
			fields, ok := base.(map[string]any)
			if !ok {
				return errors.New("path item reference must identify an object")
			}
			merged := maps.Clone(fields)
			for key, value := range item {
				if key != "$ref" {
					merged[key] = value
				}
			}
			item = merged
		}
		paths[path] = item
	}
	return nil
}

type operationContract struct {
	Parameters  []*parameter
	RequestBody *requestBody
}

type parameter struct {
	Name, In, Style   string
	Required, Explode *bool
	Schema            *schemaRef
	Content           map[string]*mediaType
}

func (p *parameter) IsDefaultFormEncoding() bool {
	return (p.Style == "" || p.Style == "form") && (p.Explode == nil || *p.Explode)
}

type requestBody struct {
	Required *bool
	Content  map[string]*mediaType
}

type mediaType struct {
	Schema   *schemaRef
	Encoding map[string]*encoding
}

type encoding struct {
	ContentType, Style         string
	Explode                    *bool
	AllowReserved, ReservedSet bool
}

type schemaRef struct {
	c       *Catalog
	pointer string
}

type schemaView struct {
	ref                              *schemaRef
	value                            any
	Type                             []string
	Format, Pattern, ContentEncoding string
	Items, PropertyNames             *schemaRef
	Properties                       map[string]*schemaRef
}

func (r *schemaRef) Schema() *schemaView {
	if r == nil {
		return nil
	}
	pointer, value, err := r.c.resolveObject(r.pointer)
	if err != nil {
		return nil
	}
	s := &schemaView{ref: r, value: value}
	if _, ok := value.(bool); ok {
		return s
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	switch types := m["type"].(type) {
	case string:
		s.Type = []string{types}
	case []any:
		for _, value := range types {
			if name, ok := value.(string); ok {
				s.Type = append(s.Type, name)
			}
		}
	}
	s.Format, _ = m["format"].(string)
	s.Pattern, _ = m["pattern"].(string)
	s.ContentEncoding, _ = m["contentEncoding"].(string)
	s.Items = r.c.schemaRef(pointer, m, "items")
	s.PropertyNames = r.c.schemaRef(pointer, m, "propertyNames")
	if properties, ok := m["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*schemaRef, len(properties))
		for name := range properties {
			s.Properties[name] = &schemaRef{r.c, pointer + "/properties/" + pointerEscape(name)}
		}
	}
	return s
}

func (c *Catalog) schemaRef(pointer string, m map[string]any, key string) *schemaRef {
	if _, ok := m[key]; !ok {
		return nil
	}
	return &schemaRef{c, pointer + "/" + pointerEscape(key)}
}

func boolPointer(value any) *bool {
	if b, ok := value.(bool); ok {
		return &b
	}
	return nil
}

func (c *Catalog) requestContract(op Operation) (*requestContract, error) {
	path, item, err := c.object("/paths/" + pointerEscape(op.Path))
	if err != nil {
		return nil, err
	}
	operation, definition, err := c.object(path + "/" + strings.ToLower(op.Method))
	if err != nil {
		return nil, err
	}
	inherited, err := c.parameters(path, item)
	if err != nil {
		return nil, err
	}
	params, err := c.parameters(operation, definition)
	if err != nil {
		return nil, err
	}
	view := &requestContract{Parameters: inherited, Operation: &operationContract{Parameters: params}}
	if _, present := definition["requestBody"]; present {
		pointer, body, err := c.object(operation + "/requestBody")
		if err != nil {
			return nil, err
		}
		content, err := c.mediaTypes(pointer+"/content", body["content"])
		if err != nil {
			return nil, err
		}
		view.Operation.RequestBody = &requestBody{Required: boolPointer(body["required"]), Content: content}
	}
	return view, nil
}

func (c *Catalog) parameters(pointer string, m map[string]any) ([]*parameter, error) {
	values, _ := m["parameters"].([]any)
	var result []*parameter
	for i := range values {
		path, value, err := c.object(pointer + "/parameters/" + strconv.Itoa(i))
		if err != nil {
			return nil, err
		}
		p := &parameter{Required: boolPointer(value["required"]), Explode: boolPointer(value["explode"]), Schema: c.schemaRef(path, value, "schema")}
		p.Name, _ = value["name"].(string)
		p.In, _ = value["in"].(string)
		p.Style, _ = value["style"].(string)
		p.Content, err = c.mediaTypes(path+"/content", value["content"])
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, nil
}

func (c *Catalog) mediaTypes(pointer string, value any) (map[string]*mediaType, error) {
	values, _ := value.(map[string]any)
	if values == nil {
		return nil, nil
	}
	result := make(map[string]*mediaType, len(values))
	for name, value := range values {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("invalid media type")
		}
		media := &mediaType{Schema: c.schemaRef(pointer+"/"+pointerEscape(name), m, "schema")}
		if encodings, ok := m["encoding"].(map[string]any); ok {
			media.Encoding = make(map[string]*encoding, len(encodings))
			for name, value := range encodings {
				v, ok := value.(map[string]any)
				if !ok {
					return nil, errors.New("invalid media encoding")
				}
				e := &encoding{Explode: boolPointer(v["explode"])}
				e.ContentType, _ = v["contentType"].(string)
				e.Style, _ = v["style"].(string)
				e.AllowReserved, _ = v["allowReserved"].(bool)
				_, e.ReservedSet = v["allowReserved"]
				media.Encoding[name] = e
			}
		}
		result[name] = media
	}
	return result, nil
}

func (c *Catalog) object(pointer string) (string, map[string]any, error) {
	pointer, value, err := c.resolveObject(pointer)
	if err != nil {
		return "", nil, err
	}
	m, ok := value.(map[string]any)
	if !ok {
		return "", nil, errors.New("reference must identify an object")
	}
	return pointer, m, nil
}

// Binding views follow local reference chains without expanding recursive graphs.
func (c *Catalog) resolveObject(pointer string) (string, any, error) {
	seen := make(map[string]bool)
	for !seen[pointer] {
		seen[pointer] = true
		value, err := pointerValue(c.value, pointer)
		if err != nil {
			return "", nil, err
		}
		m, _ := value.(map[string]any)
		ref, ok := m["$ref"].(string)
		if !ok {
			return pointer, value, nil
		}
		if !strings.HasPrefix(ref, "#/") {
			return "", nil, errors.New("binding reference must be a local JSON pointer")
		}
		pointer, err = url.PathUnescape(strings.TrimPrefix(ref, "#"))
		if err != nil {
			return "", nil, err
		}
	}
	return "", nil, errors.New("cyclic binding reference")
}

func pointerValue(value any, pointer string) (any, error) {
	if pointer == "" {
		return value, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("invalid JSON pointer")
	}
	for _, name := range strings.Split(pointer[1:], "/") {
		name = strings.ReplaceAll(strings.ReplaceAll(name, "~1", "/"), "~0", "~")
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[name]
			if !ok {
				return nil, errors.New("unresolved local reference")
			}
		case []any:
			i, err := strconv.Atoi(name)
			if err != nil || i < 0 || i >= len(v) || strconv.Itoa(i) != name {
				return nil, errors.New("invalid array reference")
			}
			value = v[i]
		default:
			return nil, errors.New("unresolved local reference")
		}
	}
	return value, nil
}

// Check reference existence during admission without constructing a second
// object model. JSON Schema identifiers establish local resource boundaries.
func validateLocalReferences(value any, kind string, resource any) error {
	return validateScopedReferences(value, kind, resource, resource)
}

func validateScopedReferences(value any, kind string, resource, document any) error {
	if kind == "opaque" {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		if kind == "schema" && v["$id"] != nil {
			resource = v
		}
		if !strings.HasPrefix(kind, "map:") {
			if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
				pointer, err := url.PathUnescape(ref[1:])
				if err != nil {
					return errors.New("invalid local reference")
				}
				if _, err = pointerValue(resource, pointer); err != nil {
					if !ignitionGenerator(document.(map[string]any)) || !strings.HasPrefix(ref, "#/components/") {
						return fmt.Errorf("unresolved local reference: %s", ref)
					}
					if _, err := pointerValue(document, pointer); err != nil {
						return err
					}
				}
			}
		}
		for name, child := range v {
			childKind := projectionChild(kind, name, child)
			if strings.HasPrefix(kind, "map:") {
				childKind = strings.TrimPrefix(kind, "map:")
			}
			if err := validateScopedReferences(child, childKind, resource, document); err != nil {
				return err
			}
		}
	case []any:
		if strings.HasPrefix(kind, "array:") {
			for _, child := range v {
				if err := validateScopedReferences(child, strings.TrimPrefix(kind, "array:"), resource, document); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Preserve exact vendor definitions for exports while indexing only route metadata.
func operationMetadata(key, path, method string, definition json.RawMessage, data map[string]any) (Operation, error) {
	op := Operation{Key: key, Path: path, Method: method, Definition: definition}
	op.OperationID, _ = data["operationId"].(string)
	op.Summary, _ = data["summary"].(string)
	op.Description, _ = data["description"].(string)
	op.Deprecated, _ = data["deprecated"].(bool)
	tags, _ := data["tags"].([]any)
	for _, tag := range tags {
		if tag, ok := tag.(string); ok {
			op.Tags = append(op.Tags, tag)
		}
	}
	return op, nil
}
