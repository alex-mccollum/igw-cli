package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"

	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/requests"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

const MaxJSONBodyBytes = 32 << 20

const ValidationSchema = "declared_schema"
const ValidationTransport = "declared_transport"

var ErrUnsupportedBodyEncoding = errors.New("the request body encoding has no supported schema decoder")

// validateBody owns presence, media selection, decoding, and coverage. It
// bypasses both lossy stages of upstream JSON body decoding:
// JSONDecoder uses float64, and Canonicalize converts json.Number to float64.
// Use the exported request-specific compiler with an exact decoded value, then
// remove only this body declaration from the remaining private validation view.
// Caller holds Catalog.validationMu because schema rendering is not read-only.
func (c *Catalog) validateBody(item *v3.PathItem, request *http.Request) (string, []Issue, error) {
	op := item.GetOperations().GetOrZero(strings.ToLower(request.Method))
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
	if op.RequestBody == nil {
		if len(raw) > 0 {
			return refuse("undeclared_body")
		}
		return ValidationSchema, nil, nil
	}
	required := op.RequestBody.Required != nil && *op.RequestBody.Required
	if len(raw) == 0 && required {
		return refuse("required")
	}
	if request.Header.Get("Content-Type") == "" {
		if len(raw) == 0 {
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
	encoding := bodyEncoding(contentType, media)
	if encoding == "opaque" || encoding == "binary" {
		return ValidationTransport, nil, nil
	}
	if len(raw) == 0 {
		return ValidationSchema, nil, nil
	}
	var value any
	switch encoding {
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		value, err = decodeUniqueJSON(decoder, 0)
		if err != nil || !jsonvalue.ValidUnicode(raw) {
			return refuse("invalid_json")
		}
		if _, err := decoder.Token(); err != io.EOF {
			return refuse("invalid_json")
		}
		if !boundedJSONNumbers(value) {
			return refuse("numeric_limit")
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
	valid, failures := requests.ValidateRequestSchema(&requests.ValidateRequestSchemaInput{
		Request: request, Schema: media.Schema.Schema(), Version: c.schemaVersion(),
		Options:      []validatorconfig.Option{validatorconfig.WithSchemaCache(nil), validatorconfig.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))},
		BodyRequired: required, DecodedValue: value, RawBody: raw, ValueDecoded: true,
	})
	if !valid {
		issues, err := validationIssues(failures)
		if err != nil || len(issues) == 0 {
			return "", nil, ErrSchemaCompilation
		}
		return "", issues, nil
	}
	return ValidationSchema, nil, nil
}

// Prefer the most specific matching range, regardless of document order.
// JSON suffixes identify decoding, not compatibility: application/json does
// not implicitly declare application/problem+json or vice versa.
func requestBodyMedia(body *v3.RequestBody, contentType string) (*v3.MediaType, bool) {
	if body.Content == nil {
		return nil, false
	}
	top, _, _ := strings.Cut(contentType, "/")
	var selected *v3.MediaType
	rank, ambiguous := 0, false
	for declared, media := range body.Content.FromOldest() {
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

func bodyEncoding(contentType string, media *v3.MediaType) string {
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
	if binaryBodySchema(media.Schema, contentType == "application/octet-stream") {
		return "binary"
	}
	return "unsupported"
}

// Raw binary has no JSON instance to validate. Recognize only the captured
// unconstrained octet-stream and legacy string/binary forms (plus annotations).
// Any additional assertion requires explicit support; never silently discard it.
// Caller holds validationMu because resolving a schema can populate caches.
func binaryBodySchema(proxy *base.SchemaProxy, allowEmpty bool) bool {
	schema := proxy.Schema()
	if schema == nil || schema.GoLow() == nil {
		return false
	}
	node := schema.GoLow().RootNode
	if node == nil || node.Tag != "!!map" {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
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
