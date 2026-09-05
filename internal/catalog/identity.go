package catalog

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const ContractPolicy = "igw-contract/1"

// Identity separates original bytes, canonical documentation, and the reviewed
// contract projection. It does not claim general JSON Schema equivalence.
type Identity struct {
	RawSHA256      string `json:"rawSha256"`
	DocumentSHA256 string `json:"documentSha256"`
	ContractSHA256 string `json:"contractSha256"`
	ContractPolicy string `json:"contractPolicy"`
}

func (c *Catalog) Identity() Identity {
	return Identity{c.rawHash, c.documentHash, c.contractHash, ContractPolicy}
}

func (c *Catalog) DocumentHash() string { return c.documentHash }

type contractProjection struct{ frozen map[string]bool }

func contractDigest(value any) (string, error) {
	p := contractProjection{frozen: make(map[string]bool)}
	p.references(value, value, "", "")
	b, err := json.Marshal(p.project(value, "document", ""))
	if err != nil {
		return "", err
	}
	return digest(append([]byte(ContractPolicy+"\n"), b...)), nil
}

func pointerChild(path, key string) string { return path + "/" + pointerEscape(key) }

// Reference targets retain their complete original subtree. Array ancestors
// must retain positions, and annotation ancestors must not be removed. Local
// fragments inside $id resources resolve relative to that resource, not the
// OpenAPI root. Unresolved/anchor/dynamic references preserve that resource.
// This intentionally errs toward extra review rather than false equivalence.
func (p contractProjection) references(value, resource any, path, resourcePath string) {
	switch v := value.(type) {
	case map[string]any:
		if _, ok := v["$id"].(string); ok {
			resource, resourcePath = v, path
		}
		for key, child := range v {
			if ref, ok := child.(string); ok && (key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" || key == "operationRef") {
				if key != "$ref" && key != "operationRef" {
					p.frozen[resourcePath] = true
				} else {
					p.referenceTarget(resource, resourcePath, ref)
				}
			}
			if key == "discriminator" {
				if discriminator, ok := child.(map[string]any); ok {
					if mapping, ok := discriminator["mapping"].(map[string]any); ok {
						for _, target := range mapping {
							if ref, ok := target.(string); ok && strings.Contains(ref, "#") {
								p.referenceTarget(resource, resourcePath, ref)
							}
						}
					}
				}
			}
			p.references(child, resource, pointerChild(path, key), resourcePath)
		}
	case []any:
		for i, child := range v {
			p.references(child, resource, pointerChild(path, strconv.Itoa(i)), resourcePath)
		}
	}
}

func (p contractProjection) referenceTarget(resource any, path, ref string) {
	if ref == "#" {
		p.frozen[path] = true
		return
	}
	fragment, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	if err != nil || !strings.HasPrefix(ref, "#") || !strings.HasPrefix(fragment, "/") {
		p.frozen[path] = true
		return
	}
	base := path
	for _, token := range strings.Split(fragment[1:], "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch v := resource.(type) {
		case map[string]any:
			var found bool
			resource, found = v[key]
			if !found {
				p.frozen[base] = true
				return
			}
		case []any:
			p.frozen[path] = true
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(v) {
				p.frozen[base] = true
				return
			}
			resource = v[i]
		default:
			p.frozen[base] = true
			return
		}
		path = pointerChild(path, key)
		// These may be annotation containers in the projection. Freezing a
		// same-named data property too is conservative and harmless.
		if documentationField(key) {
			p.frozen[path] = true
		}
	}
	p.frozen[path] = true
}

func documentationField(key string) bool {
	switch key {
	case "title", "summary", "description", "example", "examples", "externalDocs", "$comment":
		return true
	}
	return false
}

func (p contractProjection) project(value any, kind, path string) any {
	if p.frozen[path] || kind == "opaque" {
		return value
	}
	if strings.HasPrefix(kind, "map:") {
		m, ok := value.(map[string]any)
		if !ok {
			return value
		}
		out := make(map[string]any, len(m))
		for key, child := range m {
			if strings.HasPrefix(key, "x-") {
				out[key] = child
				continue
			}
			out[key] = p.project(child, strings.TrimPrefix(kind, "map:"), pointerChild(path, key))
		}
		return out
	}
	if strings.HasPrefix(kind, "array:") {
		a, ok := value.([]any)
		if !ok {
			return value
		}
		out := make([]any, len(a))
		for i, child := range a {
			out[i] = p.project(child, strings.TrimPrefix(kind, "array:"), pointerChild(path, strconv.Itoa(i)))
		}
		return out
	}
	m, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(m))
	for key, child := range m {
		childPath := pointerChild(path, key)
		if documentationField(key) && removableDocumentation(kind, key) && !p.frozen[childPath] {
			continue
		}
		childKind := projectionChild(kind, key, child)
		projected := p.project(child, childKind, childPath)
		if kind == "schema" && !p.frozen[childPath] {
			switch key {
			case "allOf", "anyOf", "oneOf", "enum", "required", "type":
				if array, ok := projected.([]any); ok {
					// Never sort arrays inside enum members or const/default data.
					keys := make([]struct {
						key   string
						value any
					}, len(array))
					for i, v := range array {
						b, _ := json.Marshal(v)
						keys[i].key, keys[i].value = string(b), v
					}
					sort.SliceStable(keys, func(i, j int) bool { return keys[i].key < keys[j].key })
					ordered := make([]any, len(keys))
					for i, v := range keys {
						ordered[i] = v.value
					}
					projected = ordered
				}
			}
		}
		out[key] = projected
	}
	return out
}

func removableDocumentation(kind, key string) bool {
	switch kind {
	case "schema":
		return key != "summary"
	case "operation", "pathItem", "parameter", "header", "media", "response", "requestBody", "link", "securityScheme":
		return key != "title" && key != "$comment"
	case "info", "tag", "server":
		return key == "description" || key == "externalDocs"
	case "document":
		return key == "externalDocs"
	}
	return false
}

func projectionChild(kind, key string, value any) string {
	switch kind {
	case "document":
		switch key {
		case "paths", "webhooks":
			return "map:pathItem"
		case "components":
			return "components"
		case "info":
			return "info"
		case "servers":
			return "array:server"
		case "tags":
			return "array:tag"
		}
	case "components":
		switch key {
		case "schemas":
			return "map:schema"
		case "parameters":
			return "map:parameter"
		case "headers":
			return "map:header"
		case "responses":
			return "map:response"
		case "requestBodies":
			return "map:requestBody"
		case "pathItems":
			return "map:pathItem"
		case "callbacks":
			return "map:map:pathItem"
		case "links":
			return "map:link"
		case "securitySchemes":
			return "map:securityScheme"
		}
	case "pathItem":
		switch key {
		case "get", "put", "post", "delete", "head", "patch", "options", "trace":
			return "operation"
		case "parameters":
			return "array:parameter"
		case "servers":
			return "array:server"
		}
	case "operation":
		switch key {
		case "parameters":
			return "array:parameter"
		case "requestBody":
			return "requestBody"
		case "responses":
			return "map:response"
		case "callbacks":
			return "map:map:pathItem"
		case "servers":
			return "array:server"
		}
	case "requestBody", "response", "parameter", "header":
		switch key {
		case "content":
			return "map:media"
		case "schema":
			return "schema"
		case "headers":
			return "map:header"
		case "links":
			return "map:link"
		}
	case "media":
		switch key {
		case "schema":
			return "schema"
		case "encoding":
			return "map:encoding"
		}
	case "encoding":
		if key == "headers" {
			return "map:header"
		}
	case "schema":
		switch key {
		case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
			return "map:schema"
		case "allOf", "anyOf", "oneOf", "prefixItems":
			return "array:schema"
		case "items":
			if _, ok := value.([]any); ok {
				return "array:schema"
			}
			return "schema"
		case "additionalItems", "additionalProperties", "unevaluatedItems", "unevaluatedProperties", "contains", "propertyNames", "not", "if", "then", "else":
			return "schema"
		}
	}
	return "opaque"
}
