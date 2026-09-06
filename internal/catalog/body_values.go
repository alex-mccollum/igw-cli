package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
)

const MaxJSONBodyBytes = 32 << 20

const ValidationSchema = "declared_schema"
const ValidationTransport = "declared_transport"

var ErrUnsupportedBodyEncoding = errors.New("the request body encoding has no supported schema decoder")

// validateBody owns body presence, media selection, exact decoding, schema
// validation, and coverage. The caller serializes schema compiler access.
func (c *Catalog) validateBody(item *requestContract, request *http.Request) (string, []Issue, error) {
	op := item.Operation
	refuse := func(rule string) (string, []Issue, error) {
		return "", []Issue{{Kind: "requestBody", Rule: rule}}, nil
	}
	raw, err := readValidationBody(request)
	if err != nil {
		return refuse("body_read")
	}
	if len(raw) > MaxJSONBodyBytes {
		return refuse("body_limit")
	}
	// Validation receives outgoing client requests: nil means omitted, while
	// http.NoBody (or an independent empty reader) is an explicit empty input.
	present := request.Body != nil
	if op.RequestBody == nil {
		if present {
			return refuse("undeclared_body")
		}
		return ValidationSchema, nil, nil
	}
	required := op.RequestBody.Required != nil && *op.RequestBody.Required
	if !present && required {
		return refuse("required")
	}
	if request.Header.Get("Content-Type") == "" {
		if !present {
			return ValidationSchema, nil, nil
		}
		return refuse("content_type")
	}
	contentType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.Contains(contentType, "/") || strings.Contains(contentType, "*") {
		return refuse("content_type")
	}
	media, ambiguous := requestBodyMedia(op.RequestBody, contentType)
	if ambiguous {
		return refuse("ambiguous_media_type")
	}
	if media == nil {
		return refuse("content_type")
	}
	if !present {
		return ValidationSchema, nil, nil
	}
	encoding := bodyEncoding(contentType, media)
	if encoding == "opaque" || encoding == "binary" {
		return ValidationTransport, nil, nil
	}
	var value any
	switch encoding {
	case "urlencoded":
		var rule string
		value, rule = decodeFormBody(raw, media, parameters["charset"])
		if rule == "unsupported_serialization" {
			return "", nil, ErrUnsupportedBodyEncoding
		}
		if rule != "" {
			return refuse(rule)
		}
	case "json":
		var rule string
		value, rule = decodeExactJSON(raw)
		if rule != "" {
			return refuse(rule)
		}
	case "utf8":
		charset := strings.ToLower(parameters["charset"])
		if charset != "" && charset != "utf-8" && charset != "us-ascii" {
			return "", nil, ErrUnsupportedBodyEncoding
		}
		if !utf8.Valid(raw) {
			return refuse("invalid_text")
		}
		if charset == "us-ascii" {
			for _, b := range raw {
				if b >= utf8.RuneSelf {
					return refuse("invalid_text")
				}
			}
		}
		value = string(raw)
	default:
		return "", nil, ErrUnsupportedBodyEncoding
	}
	if media.Schema.Schema() == nil {
		return "", nil, ErrSchemaCompilation
	}
	issues, err := c.validateSchema(media.Schema.Schema(), value, true)
	if err != nil || len(issues) > 0 {
		return "", issues, err
	}
	return ValidationSchema, nil, nil
}

// Preserve exact numbers, null, member uniqueness, and Unicode for every JSON
// input location. Callers bound the encoded input before decoding.
func decodeExactJSON(raw []byte) (any, string) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueJSON(decoder, 0)
	if err != nil || !jsonvalue.ValidUnicode(raw) {
		return nil, "invalid_json"
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, "invalid_json"
	}
	if !boundedJSONNumbers(value) {
		return nil, "numeric_limit"
	}
	return value, ""
}

// Prefer the most specific matching range, regardless of document order.
// JSON suffixes identify decoding, not compatibility: application/json does
// not implicitly declare application/problem+json or vice versa.
func requestBodyMedia(body *requestBody, contentType string) (*mediaType, bool) {
	if body.Content == nil {
		return nil, false
	}
	top, _, _ := strings.Cut(contentType, "/")
	var selected *mediaType
	rank, ambiguous := 0, false
	for declared, media := range body.Content {
		normalized := strings.ToLower(declared)
		current := 0
		switch normalized {
		case contentType:
			current = 3
		case top + "/*":
			current = 2
		case "*/*":
			current = 1
		}
		if current == 0 || current < rank {
			continue
		}
		if current == rank {
			ambiguous = true
		} else {
			selected, rank, ambiguous = media, current, false
		}
	}
	return selected, ambiguous
}

func bodyEncoding(contentType string, media *mediaType) string {
	if media.Schema == nil {
		return "opaque"
	}
	_, subtype, _ := strings.Cut(contentType, "/")
	if subtype == "json" || strings.HasSuffix(subtype, "+json") {
		return "json"
	}
	if contentType == "text/plain" {
		return "utf8"
	}
	if contentType == "application/x-www-form-urlencoded" {
		return "urlencoded"
	}
	if binaryBodySchema(media.Schema, contentType == "application/octet-stream") {
		return "binary"
	}
	return "unsupported"
}

// Raw binary has no JSON instance to validate. Recognize only the captured
// unconstrained octet-stream and legacy string/binary forms (plus annotations).
// Any additional assertion requires explicit support; never silently discard it.
// Schema binding views resolve only local references.
func binaryBodySchema(proxy *schemaRef, allowEmpty bool) bool {
	schema := proxy.Schema()
	if schema == nil {
		return false
	}
	fields, ok := schema.value.(map[string]any)
	if !ok {
		return false
	}
	for key := range fields {
		switch key {
		case "title", "description", "example", "examples", "$comment", "deprecated", "readOnly", "writeOnly", "default":
		case "type":
			if len(schema.Type) != 1 || schema.Type[0] != "string" {
				return false
			}
		case "format":
			if schema.Format != "binary" {
				return false
			}
		default:
			return false
		}
	}
	return (schema.Format == "binary" && len(schema.Type) == 1 && schema.Type[0] == "string") ||
		(allowEmpty && len(schema.Type) == 0 && schema.Format == "")
}

func readValidationBody(request *http.Request) ([]byte, error) {
	if request.Body == nil || request.Body == http.NoBody {
		return nil, nil
	}
	reader := request.Body
	if request.GetBody != nil {
		var err error
		reader, err = request.GetBody()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
	}
	return io.ReadAll(io.LimitReader(reader, MaxJSONBodyBytes+1))
}

func boundedJSONNumbers(value any) bool {
	switch value := value.(type) {
	case json.Number:
		_, rule := parameterPrimitive("number", string(value))
		return rule == ""
	case map[string]any:
		for _, child := range value {
			if !boundedJSONNumbers(child) {
				return false
			}
		}
	case []any:
		for _, child := range value {
			if !boundedJSONNumbers(child) {
				return false
			}
		}
	}
	return true
}
