// Package catalog owns vendor OpenAPI parsing and validation. Its public types
// do not expose parser-specific models; raw vendor bytes remain authoritative.
package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/schema_validation"
	"github.com/pb33f/libopenapi/datamodel"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

const MaxDocumentBytes = 32 << 20
const ParserVersion = "libopenapi/0.38.7+validator/0.14.0;igw/9"

var ErrSchemaCompilation = errors.New("the Gateway's operation schema cannot be compiled")
var ErrIncompleteContract = errors.New("the Gateway's operation has an undocumented input schema")

type Operation struct {
	Key         string          `json:"key"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	OperationID string          `json:"operationId,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	Description string          `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Deprecated  bool            `json:"deprecated,omitempty"`
	Definition  json.RawMessage `json:"definition"`
}

// Description includes the containing path item and shared components so that
// inherited parameters, security, and referenced schemas remain inspectable.
type Description struct {
	Operation   Operation       `json:"operation"`
	PathItem    json.RawMessage `json:"pathItem"`
	Components  json.RawMessage `json:"components,omitempty"`
	Security    json.RawMessage `json:"security,omitempty"`
	Gaps        []string        `json:"gaps,omitempty"`
	Adjustments []Adjustment    `json:"adjustments,omitempty"`
}

type Issue struct {
	Kind      string `json:"kind"`
	Rule      string `json:"rule,omitempty"`
	Parameter string `json:"parameter,omitempty"`
	Field     string `json:"field,omitempty"`
	Schema    string `json:"schema,omitempty"`
}

type Catalog struct {
	raw          []byte
	root         map[string]json.RawMessage
	paths        map[string]map[string]json.RawMessage
	ops          map[string]Operation
	aliases      map[string][]string
	document     libopenapi.Document
	model        *libopenapi.DocumentModel[v3.Document]
	once         sync.Once
	validator    validator.Validator
	rawHash      string
	documentHash string
	contractHash string
	adjustments  []Adjustment
}

func Parse(raw []byte) (*Catalog, error) {
	if len(raw) == 0 || len(raw) > MaxDocumentBytes {
		return nil, fmt.Errorf("OpenAPI document must contain 1 to %d bytes", MaxDocumentBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueJSON(decoder, 0)
	if err != nil {
		return nil, errors.New("OpenAPI document must be valid JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("OpenAPI document contains trailing data")
	}
	if err := checkReferences(value); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	contractHash, err := contractDigest(value)
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, errors.New("OpenAPI root must be an object")
	}
	var version string
	_ = json.Unmarshal(root["openapi"], &version)
	if !strings.HasPrefix(version, "3.0.") && !strings.HasPrefix(version, "3.1.") {
		return nil, errors.New("supported OpenAPI versions are 3.0 and 3.1")
	}
	adjustments := normalizeIgnition(value)
	// The parser's node index scans each line's nodes linearly. Indent its
	// private input to avoid quadratic work on compact multi-megabyte JSON.
	modelBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	doc, err := libopenapi.NewDocumentWithConfiguration(modelBytes, &datamodel.DocumentConfiguration{
		AllowFileReferences: false, AllowRemoteReferences: false,
		SkipExternalRefResolution: true,
		// Recursive arrays can terminate with an empty array, including IA's
		// required SecurityLevelRuleNode.children. Validate actual values later.
		IgnoreArrayCircularReferences: true,
		Logger:                        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, errors.New("invalid OpenAPI document structure")
	}
	valid, _ := schema_validation.ValidateOpenAPIDocument(doc)
	if !valid {
		doc.Release()
		return nil, errors.New("document does not satisfy its OpenAPI version schema")
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		doc.Release()
		return nil, errors.New("OpenAPI references or model could not be resolved")
	}
	c := &Catalog{
		raw: bytes.Clone(raw), root: root, document: doc, model: model,
		ops: make(map[string]Operation), aliases: make(map[string][]string),
		rawHash: digest(raw), documentHash: digest(canonical), contractHash: contractHash,
		adjustments: adjustments,
	}
	if err := json.Unmarshal(root["paths"], &c.paths); err != nil {
		c.Close()
		return nil, errors.New("OpenAPI paths must be an object")
	}
	if model.Model.Paths != nil {
		for path, item := range model.Model.Paths.PathItems.FromOldest() {
			definitions, err := c.pathDefinitions(path)
			if err != nil {
				c.Close()
				return nil, err
			}
			for method, op := range item.GetOperations().FromOldest() {
				upper := strings.ToUpper(method)
				key := upper + " " + path
				c.ops[key] = Operation{Key: key, Method: upper, Path: path, OperationID: op.OperationId,
					Summary: op.Summary, Description: op.Description, Tags: op.Tags,
					Deprecated: op.Deprecated != nil && *op.Deprecated,
					Definition: definitions[method],
				}
				if op.OperationId != "" {
					c.aliases[op.OperationId] = append(c.aliases[op.OperationId], key)
				}
			}
		}
	}
	return c, nil
}

func (c *Catalog) pathDefinitions(path string) (map[string]json.RawMessage, error) {
	item := c.paths[path]
	seen := make(map[string]bool)
	for item["$ref"] != nil {
		var ref string
		if err := json.Unmarshal(item["$ref"], &ref); err != nil || !strings.HasPrefix(ref, "#/") || seen[ref] {
			return nil, errors.New("path item reference must be a resolvable local JSON pointer")
		}
		seen[ref] = true
		var raw json.RawMessage
		fields := c.root
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			raw = fields[part]
			var next map[string]json.RawMessage
			if err := json.Unmarshal(raw, &next); err != nil {
				return nil, errors.New("path item reference could not be inspected")
			}
			fields = next
		}
		resolved := make(map[string]json.RawMessage)
		if err := json.Unmarshal(raw, &resolved); err != nil {
			return nil, err
		}
		for key, value := range item {
			if key != "$ref" {
				resolved[key] = value
			}
		}
		item = resolved
	}
	return item, nil
}

func checkReferences(value any) error {
	return walkReferences(value, false)
}

// A property name can itself be "$ref" (SCIM uses this). Map keys in schema
// property/definition collections are names, while the values remain schemas
// whose own reference and dialect keywords must still be checked.
func walkReferences(value any, names bool) error {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if names {
				if err := walkReferences(child, false); err != nil {
					return err
				}
				continue
			}
			if key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return errors.New("external OpenAPI references are disabled; import a self-contained document")
				}
			}
			if key == "$schema" || key == "jsonSchemaDialect" {
				dialect, ok := child.(string)
				defaultOAS := key == "jsonSchemaDialect" && dialect == "https://spec.openapis.org/oas/3.1/dialect/base"
				if !ok || (!defaultOAS && !embeddedDialect(dialect)) {
					return errors.New("custom JSON Schema dialects are disabled to prevent external schema retrieval")
				}
			}
			nameMap := key == "properties" || key == "patternProperties" || key == "$defs" || key == "definitions" || key == "schemas"
			if err := walkReferences(child, nameMap); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := checkReferences(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func embeddedDialect(uri string) bool {
	switch strings.TrimSuffix(uri, "#") {
	case "https://json-schema.org/draft/2020-12/schema", "https://json-schema.org/draft/2019-09/schema",
		"http://json-schema.org/draft-07/schema", "http://json-schema.org/draft-06/schema", "http://json-schema.org/draft-04/schema":
		return true
	default:
		return false
	}
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func (c *Catalog) Raw() []byte     { return bytes.Clone(c.raw) }
func (c *Catalog) RawHash() string { return c.rawHash }

// ContractHash hashes the versioned, reference-preserving contract projection.
// DocumentHash retains every field for documentation/representation drift.
func (c *Catalog) ContractHash() string { return c.contractHash }

func (c *Catalog) OperationCount() int { return len(c.ops) }

func (c *Catalog) Operations() []Operation {
	out := make([]Operation, 0, len(c.ops))
	for _, op := range c.ops {
		out = append(out, cloneOperation(op))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func cloneOperation(op Operation) Operation {
	op.Definition = bytes.Clone(op.Definition)
	op.Tags = append([]string(nil), op.Tags...)
	return op
}

func (c *Catalog) Resolve(keyOrAlias string) (Operation, error) {
	if op, ok := c.ops[keyOrAlias]; ok {
		return cloneOperation(op), nil
	}
	keys := c.aliases[keyOrAlias]
	if len(keys) > 1 {
		return Operation{}, errors.New("operationId is ambiguous; select an exact METHOD /path key")
	}
	if len(keys) == 1 {
		return cloneOperation(c.ops[keys[0]]), nil
	}
	return Operation{}, errors.New("operation not found; list the target Gateway catalog")
}

func (c *Catalog) Describe(keyOrAlias string) (Description, error) {
	op, err := c.Resolve(keyOrAlias)
	if err != nil {
		return Description{}, err
	}
	pathItem, err := json.Marshal(c.paths[op.Path])
	if err != nil {
		return Description{}, err
	}
	description := Description{Operation: op, PathItem: pathItem, Components: bytes.Clone(c.root["components"]),
		Security: bytes.Clone(c.root["security"]), Gaps: c.gaps(op)}
	for _, adjustment := range c.adjustments {
		if adjustment.Operation == op.Key {
			description.Adjustments = append(description.Adjustments, adjustment)
			switch adjustment.Rule {
			case "empty-responses":
				description.Gaps = append(description.Gaps, "The Gateway declares no response contract for this operation.")
			case "selected-path-required":
				description.Gaps = append(description.Gaps, "Supply every placeholder in the selected path; the vendor's optional path form requires a separate explicit request.")
			case "script-cancel-undocumented-id":
				description.Gaps = append(description.Gaps, "The Gateway omits the id parameter's schema; schema-assisted requests and previews are unavailable for this operation.")
			}
		}
	}
	return description, nil
}

func (c *Catalog) gaps(op Operation) []string {
	item := c.model.Model.Paths.PathItems.GetOrZero(op.Path)
	definition := item.GetOperations().GetOrZero(strings.ToLower(op.Method))
	if op.Method != "GET" && op.Method != "HEAD" && definition.RequestBody == nil {
		return []string{"No request body contract is declared; server validation is still required."}
	}
	if body := definition.RequestBody; body != nil && body.Content != nil {
		for _, media := range body.Content.FromOldest() {
			if media.Schema == nil {
				return []string{"A request media type has no schema; input validation is incomplete."}
			}
		}
	}
	return nil
}

// Validate checks a credential-free request. The caller retains responsibility
// for target selection, authorization, workflow effects, and server validation.
// Request bodies must be independent readers: the validator may consume them.
func (c *Catalog) Validate(key string, request *http.Request) ([]Issue, error) {
	op, err := c.Resolve(key)
	if err != nil {
		return nil, err
	}
	if request.Method != op.Method {
		return nil, errors.New("request method differs from selected operation")
	}
	for _, adjustment := range c.adjustments {
		if adjustment.Operation == op.Key && adjustment.Rule == "script-cancel-undocumented-id" {
			return nil, ErrIncompleteContract
		}
	}
	c.once.Do(func() {
		// The default eagerly compiles every request/response schema. A CLI
		// invocation needs only the selected contract, and unrelated vendor
		// schema defects must not interfere with that operation.
		c.validator = validator.NewValidatorFromV3Model(&c.model.Model, validatorconfig.WithoutSecurityValidation(), validatorconfig.WithSchemaCache(nil))
	})
	item := c.model.Model.Paths.PathItems.GetOrZero(op.Path)
	item, bindingIssue := listValidationView(item, request, op.Path)
	if bindingIssue != nil {
		return []Issue{*bindingIssue}, nil
	}
	valid, failures := c.validator.ValidateHttpRequestSyncWithPathItem(request, item, op.Path)
	if valid {
		return nil, nil
	}
	issues := make([]Issue, 0, len(failures))
	for _, failure := range failures {
		if len(failure.SchemaValidationErrors) == 0 && strings.Contains(failure.Message, "failed schema compilation") {
			return nil, ErrSchemaCompilation
		}
		issue := Issue{Kind: failure.ValidationType, Rule: failure.ValidationSubType, Parameter: failure.ParameterName}
		if len(failure.SchemaValidationErrors) == 0 {
			issues = append(issues, issue)
		}
		for _, field := range failure.SchemaValidationErrors {
			copy := issue
			copy.Field, copy.Schema = field.FieldPath, field.KeywordLocation
			issues = append(issues, copy)
		}
	}
	return issues, nil
}

// Close requires all users of the catalog to have finished.
func (c *Catalog) Close() {
	if c.validator != nil {
		c.validator.Release()
	}
	if c.document != nil {
		c.document.Release()
	}
}
