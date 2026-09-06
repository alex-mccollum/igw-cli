package catalog

import (
	"bytes"
	"cmp"
	"embed"
	"errors"
	"maps"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

//go:embed schemas/*.json
var documentSchemas embed.FS

const schemaDocumentURL = "https://igw.invalid/contract"

type offlineSchemaLoader struct{}

func (offlineSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("external schema loading is disabled")
}

func newSchemaCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(offlineSchemaLoader{})
	// Match the existing OpenAPI keyword checks without coercing wire values.
	c.RegisterVocabulary(&jsonschema.Vocabulary{URL: "urn:igw:openapi", Compile: compileOpenAPIKeywords})
	c.AssertVocabs()
	return c
}

// JSON Schema handles assertions and standard annotations. These are the
// additional OpenAPI checks previously provided by the request validator.
func compileOpenAPIKeywords(_ *jsonschema.CompilerContext, schema map[string]any) (jsonschema.SchemaExt, error) {
	if _, exists := schema["nullable"]; exists {
		return nil, errors.New("nullable requires OpenAPI 3.0 normalization")
	}
	if v, exists := schema["deprecated"]; exists {
		if _, ok := v.(bool); !ok {
			return nil, errors.New("deprecated must be boolean")
		}
	}
	value, exists := schema["discriminator"]
	if !exists {
		return nil, nil
	}
	d, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("discriminator must be an object")
	}
	property, ok := d["propertyName"].(string)
	if !ok {
		return nil, errors.New("discriminator must name its property")
	}
	if value, exists := d["mapping"]; exists {
		mapping, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("discriminator mapping must be an object")
		}
		for _, value := range mapping {
			if _, ok := value.(string); !ok {
				return nil, errors.New("discriminator mapping must contain strings")
			}
		}
	}
	return discriminatorProperty(property), nil
}

type discriminatorProperty string

func (property discriminatorProperty) Validate(ctx *jsonschema.ValidatorContext, value any) {
	if property == "" {
		return
	}
	m, _ := value.(map[string]any)
	if _, ok := m[string(property)]; !ok {
		ctx.AddError(&kind.Required{Missing: []string{string(property)}})
	}
}

var documentSchema30 = sync.OnceValues(func() (*jsonschema.Schema, error) { return loadDocumentSchema("oas3-schema.json") })
var documentSchema31 = sync.OnceValues(func() (*jsonschema.Schema, error) { return loadDocumentSchema("oas31-schema.json") })

func loadDocumentSchema(name string) (*jsonschema.Schema, error) {
	raw, err := documentSchemas.ReadFile("schemas/" + name)
	if err != nil {
		return nil, err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := newSchemaCompiler()
	if err := compiler.AddResource(schemaDocumentURL, value); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaDocumentURL)
}

// Caller serializes compiler access; compiled schemas are immutable. Parameter
// and request-body compilers differ only in directional required properties.
func (c *Catalog) validateSchema(schema *schemaView, value any, body bool) ([]Issue, error) {
	if schema == nil {
		return nil, ErrSchemaCompilation
	}
	index := 0
	if body {
		index = 1
	}
	compiler := c.compilers[index]
	if compiler == nil {
		compiler = newSchemaCompiler()
		document, _ := c.compilationValue(c.value, "document", body)
		if err := compiler.AddResource(schemaDocumentURL, document); err != nil {
			return nil, ErrSchemaCompilation
		}
		c.compilers[index] = compiler
	}
	compiled, err := compiler.Compile(schemaDocumentURL + "#" + url.PathEscape(schema.ref.pointer))
	if err != nil {
		return nil, ErrSchemaCompilation
	}
	if err = compiled.Validate(value); err == nil {
		return nil, nil
	}
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return nil, ErrSchemaCompilation
	}
	var issues []Issue
	var visit func(*jsonschema.ValidationError)
	visit = func(failure *jsonschema.ValidationError) {
		if len(failure.Causes) > 0 {
			for _, child := range failure.Causes {
				visit(child)
			}
			return
		}
		issue := Issue{Kind: "requestBody", Rule: "schema"}
		for _, token := range failure.InstanceLocation {
			issue.Field += "/" + pointerEscape(token)
		}
		issue.Schema = failure.SchemaURL
		issues = append(issues, issue)
	}
	visit(validation)
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		return cmp.Or(cmp.Compare(a.Field, b.Field), cmp.Compare(a.Schema, b.Schema)) < 0
	})
	return issues, nil
}

// The compiler sees the original tree plus changed branches only. Request
// bodies omit read-only properties from required lists; parameters do not.
func (c *Catalog) compilationValue(value any, position string, body bool) (any, bool) {
	if position == "opaque" {
		return value, false
	}
	switch v := value.(type) {
	case map[string]any:
		var out map[string]any
		set := func(key string, value any) {
			if out == nil {
				out = maps.Clone(v)
			}
			out[key] = value
		}
		for key, child := range v {
			childPosition := projectionChild(position, key, child)
			if strings.HasPrefix(position, "map:") {
				childPosition = strings.TrimPrefix(position, "map:")
			}
			if changed, ok := c.compilationValue(child, childPosition, body); ok {
				set(key, changed)
			}
		}
		if position == "schema" {
			// IA's component pointers are rooted in the OpenAPI document even
			// inside resource schemas carrying a relative identifier. Rebase
			// them to our registered in-memory resource, never a network URL.
			if ref, ok := v["$ref"].(string); ok && ignitionGenerator(c.value) && strings.HasPrefix(ref, "#/components/") {
				set("$ref", schemaDocumentURL+ref)
			}
			if body {
				if required, ok := v["required"].([]any); ok {
					properties, _ := v["properties"].(map[string]any)
					kept := make([]any, 0, len(required))
					for _, name := range required {
						key, _ := name.(string)
						if !c.readOnlySchema(properties[key], make(map[string]bool)) {
							kept = append(kept, name)
						}
					}
					if len(kept) != len(required) {
						set("required", kept)
						if len(kept) == 0 {
							delete(out, "required")
						}
					}
				}
			}
		}
		if out != nil {
			return out, true
		}
	case []any:
		if strings.HasPrefix(position, "array:") {
			var out []any
			for i, child := range v {
				if changed, ok := c.compilationValue(child, strings.TrimPrefix(position, "array:"), body); ok {
					if out == nil {
						out = append([]any(nil), v...)
					}
					out[i] = changed
				}
			}
			if out != nil {
				return out, true
			}
		}
	}
	return value, false
}

func (c *Catalog) readOnlySchema(value any, seen map[string]bool) bool {
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if readOnly, _ := m["readOnly"].(bool); readOnly {
		return true
	}
	if ref, ok := m["$ref"].(string); ok && strings.HasPrefix(ref, "#/") && !seen[ref] {
		seen[ref] = true
		defer delete(seen, ref)
		pointer, err := url.PathUnescape(ref[1:])
		if err == nil {
			value, err := pointerValue(c.value, pointer)
			if err == nil && c.readOnlySchema(value, seen) {
				return true
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		values, _ := m[key].([]any)
		for _, v := range values {
			if c.readOnlySchema(v, seen) {
				return true
			}
		}
	}
	return false
}
