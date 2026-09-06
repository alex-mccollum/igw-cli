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
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const MaxDocumentBytes = 32 << 20
const ParserVersion = "jsonschema/6.0.2;igw/23"

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
	BodyInputs  []BodyInput     `json:"bodyInputs,omitempty"`
	Document    json.RawMessage `json:"document,omitempty"`
}

type Issue struct {
	Kind      string `json:"kind"`
	Rule      string `json:"rule,omitempty"`
	Parameter string `json:"parameter,omitempty"`
	Field     string `json:"field,omitempty"`
	Schema    string `json:"schema,omitempty"`
}

// ValidationResult reports actual checks for the supplied request. Coverage is
// empty on failure; a declared transport is not schema validation of its bytes.
type ValidationResult struct {
	Coverage string
	Issues   []Issue
}

type Catalog struct {
	raw          []byte
	root         map[string]json.RawMessage
	paths        map[string]map[string]json.RawMessage
	ops          map[string]Operation
	aliases      map[string][]string
	value        map[string]any
	version      string
	validationMu sync.Mutex
	compilers    [2]*jsonschema.Compiler
	rawHash      string
	documentHash string
	contractHash string
	adjustments  []Adjustment
}

func Parse(raw []byte) (*Catalog, error) {
	if len(raw) == 0 || len(raw) > MaxDocumentBytes {
		return nil, fmt.Errorf("OpenAPI document must contain 1 to %d bytes", MaxDocumentBytes)
	}
	raw = bytes.Clone(raw)
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
	documentHash, err := canonicalDigest(value, "")
	if err != nil {
		return nil, err
	}
	contractHash, err := contractDigest(value)
	if err != nil {
		return nil, err
	}
	document, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("OpenAPI root must be an object")
	}
	version, _ := document["openapi"].(string)
	// Decode raw operation definitions directly, avoiding an intermediate copy
	// of the entire paths object. Only reference-bearing path items need a
	// general raw-root index for their original sibling/definition bytes.
	var index struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components json.RawMessage                       `json:"components"`
		Security   json.RawMessage                       `json:"security"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, errors.New("invalid OpenAPI index")
	}
	root := map[string]json.RawMessage{"components": index.Components, "security": index.Security}
	for _, item := range index.Paths {
		if item["$ref"] != nil {
			if err := json.Unmarshal(raw, &root); err != nil {
				return nil, err
			}
			break
		}
	}
	if !strings.HasPrefix(version, "3.0.") && !strings.HasPrefix(version, "3.1.") {
		return nil, errors.New("supported OpenAPI versions are 3.0 and 3.1")
	}
	adjustments := normalizeIgnition(value)
	if !validDocumentValue(value, version) {
		return nil, errors.New("document does not satisfy its OpenAPI version schema or numeric work limits")
	}
	if strings.HasPrefix(version, "3.0.") {
		normalizeSchema30(value, "document")
	}
	if err := validateLocalReferences(value, "document", value); err != nil {
		return nil, err
	}
	c := &Catalog{
		raw: raw, root: root, paths: index.Paths, value: value.(map[string]any), version: version,
		ops: make(map[string]Operation), aliases: make(map[string][]string),
		rawHash: digest(raw), documentHash: documentHash, contractHash: contractHash,
		adjustments: adjustments,
	}
	if err := c.expandPathItems(); err != nil {
		return nil, err
	}
	for path := range c.paths {
		definitions, err := c.pathDefinitions(path)
		if err != nil {
			return nil, err
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"} {
			definition, present := definitions[method]
			if !present {
				continue
			}
			upper := strings.ToUpper(method)
			key := upper + " " + path
			_, item, err := c.object("/paths/" + pointerEscape(path))
			if err != nil {
				return nil, err
			}
			metadata, _ := item[method].(map[string]any)
			op, err := operationMetadata(key, path, upper, definition, metadata)
			if err != nil {
				return nil, err
			}
			c.ops[key] = op
			if op.OperationID != "" {
				c.aliases[op.OperationID] = append(c.aliases[op.OperationID], key)
			}
		}
	}
	delete(c.root, "paths") // The path index already owns these definitions.
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
	return c.describe(keyOrAlias, true)
}

func (c *Catalog) DescribeCompact(keyOrAlias string) (Description, error) {
	return c.describe(keyOrAlias, false)
}

func (c *Catalog) describe(keyOrAlias string, full bool) (Description, error) {
	op, err := c.Resolve(keyOrAlias)
	if err != nil {
		return Description{}, err
	}
	pathItem, err := json.Marshal(c.paths[op.Path])
	if err != nil {
		return Description{}, err
	}
	description := Description{Operation: op, PathItem: pathItem,
		Security: bytes.Clone(c.root["security"]), Gaps: c.gaps(op), BodyInputs: c.bodyInputs(op)}
	if full {
		description.Components = bytes.Clone(c.root["components"])
	} else if err := c.compactDescription(&description); err != nil {
		return Description{}, err
	}
	reported := make(map[string]bool)
	for _, adjustment := range c.adjustments {
		if adjustment.Operation == op.Key {
			description.Adjustments = append(description.Adjustments, adjustment)
			if reported[adjustment.Rule] {
				continue
			}
			reported[adjustment.Rule] = true
			switch adjustment.Rule {
			case "empty-responses":
				description.Gaps = append(description.Gaps, "The Gateway declares no response contract for this operation.")
			case "selected-path-required":
				description.Gaps = append(description.Gaps, "Supply every placeholder in the selected path; the vendor's optional path form requires a separate explicit request.")
			case "script-cancel-undocumented-id":
				description.Gaps = append(description.Gaps, "The Gateway omits the id parameter's schema; schema-assisted requests and previews are unavailable for this operation.")
			case "sfc-undocumented-path":
				description.Gaps = append(description.Gaps, "The Gateway omits the SFC path parameter schemas; schema-assisted requests and previews are unavailable for this operation.")
			case "keyboard-local-definitions":
				description.Gaps = append(description.Gaps, "The Gateway's keyboard reference paths do not resolve; validation expands its embedded definitions without changing their value constraints.")
			}
		}
	}
	return description, nil
}

func (c *Catalog) gaps(op Operation) []string {
	item, err := c.requestContract(op)
	if err != nil {
		return []string{"The operation contract could not be resolved."}
	}
	definition := item.Operation
	if op.Method != "GET" && op.Method != "HEAD" && definition.RequestBody == nil {
		return []string{"No request body contract is declared; use api raw explicitly to send a body."}
	}
	if body := definition.RequestBody; body != nil && body.Content != nil {
		for _, media := range body.Content {
			if media.Schema == nil {
				return []string{"A request media type has no schema; only transport checks are available for that encoding."}
			}
		}
	}
	return nil
}

// Validate checks a credential-free request. The caller retains responsibility
// for target selection, authorization, workflow effects, and server validation.
// Request bodies must be independent readers: the validator may consume them.
// Use outgoing client requests: nil Body omits input; a non-nil reader,
// including http.NoBody, explicitly supplies a representation, possibly empty.
// X-Ignition-API-Token is presence-only: pass a marker, never the credential.
// The Gateway owns authentication; token value assertions are not evaluated.
// Concurrent calls are supported; schema compilation within one catalog is
// serialized while compiling and caching selected schemas.
func (c *Catalog) Validate(key string, request *http.Request) ([]Issue, error) {
	result, err := c.ValidateRequest(key, request)
	return result.Issues, err
}

func (c *Catalog) ValidateRequest(key string, request *http.Request) (ValidationResult, error) {
	issues, coverage, err := c.validateRequest(key, request)
	return ValidationResult{Coverage: coverage, Issues: issues}, err
}

func (c *Catalog) validateRequest(key string, request *http.Request) ([]Issue, string, error) {
	op, err := c.Resolve(key)
	if err != nil {
		return nil, "", err
	}
	if request.Method != op.Method {
		return nil, "", errors.New("request method differs from selected operation")
	}
	for _, adjustment := range c.adjustments {
		if adjustment.Operation == op.Key && (adjustment.Rule == "script-cancel-undocumented-id" || adjustment.Rule == "sfc-undocumented-path") {
			return nil, "", ErrIncompleteContract
		}
	}
	c.validationMu.Lock()
	defer c.validationMu.Unlock()
	item, err := c.requestContract(op)
	if err != nil {
		return nil, "", ErrIncompleteContract
	}
	cookies := make(map[string]*parameter)
	for _, list := range [][]*parameter{item.Parameters, item.Operation.Parameters} {
		for _, p := range list {
			if p.In == "cookie" {
				cookies[p.Name] = p
			}
		}
	}
	cookieNames := make([]string, 0, len(cookies))
	for name := range cookies {
		cookieNames = append(cookieNames, name)
	}
	sort.Strings(cookieNames)
	for _, name := range cookieNames {
		p := cookies[name]
		if p.Required != nil && *p.Required {
			return []Issue{{Kind: "parameter", Rule: "required", Parameter: p.Name}}, "", nil
		}
	}
	item, bindingIssues, err := c.pathValidationView(item, request, op.Path)
	if err != nil || len(bindingIssues) != 0 {
		return bindingIssues, "", err
	}
	item, request, bindingIssues, err = c.filterValidationView(item, request)
	if err != nil || len(bindingIssues) != 0 {
		return bindingIssues, "", err
	}
	item, request, bindingIssues, err = c.namedQueryValidationView(item, request)
	if err != nil || len(bindingIssues) != 0 {
		return bindingIssues, "", err
	}
	item, request, bindingIssues, err = c.headerValidationView(item, request)
	if err != nil || len(bindingIssues) != 0 {
		return bindingIssues, "", err
	}
	coverage, bindingIssues, err := c.validateBody(item, request)
	if err != nil || len(bindingIssues) != 0 {
		return bindingIssues, "", err
	}
	// All declared inputs have been bound and validated above.
	return nil, coverage, nil
}

// Close requires all users of the catalog to have finished.
func (c *Catalog) Close() {}
