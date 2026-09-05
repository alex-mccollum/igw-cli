package catalog

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/requests"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

const MaxJSONBodyBytes = 32 << 20

// jsonBodyValidationView bypasses both lossy stages of upstream body decoding:
// JSONDecoder uses float64, and Canonicalize converts json.Number to float64.
// Use the exported request-specific compiler with an exact decoded value, then
// remove only this body declaration from the remaining private validation view.
// Caller holds Catalog.validationMu because schema rendering is not read-only.
func (c *Catalog) jsonBodyValidationView(item *v3.PathItem, request *http.Request) (*v3.PathItem, *http.Request, []Issue, error) {
	op := item.GetOperations().GetOrZero(strings.ToLower(request.Method))
	if op == nil || op.RequestBody == nil || request.Header.Get("Content-Type") == "" {
		return item, request, nil, nil
	}
	refuse := func(rule string) (*v3.PathItem, *http.Request, []Issue, error) {
		return item, request, []Issue{{Kind: "requestBody", Rule: rule}}, nil
	}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	_, subtype, ok := strings.Cut(contentType, "/")
	if err != nil || !ok {
		return refuse("content_type")
	}
	if subtype != "json" && !strings.HasSuffix(subtype, "+json") {
		return item, request, nil, nil
	}
	media, ambiguous := jsonBodyMedia(op.RequestBody, contentType)
	if ambiguous {
		return refuse("ambiguous_media_type")
	}
	if media == nil {
		return refuse("content_type")
	}
	raw, err := readValidationBody(request)
	if err != nil {
		return refuse("body_read")
	}
	if len(raw) > MaxJSONBodyBytes {
		return refuse("body_limit")
	}
	required := op.RequestBody.Required != nil && *op.RequestBody.Required
	if len(raw) == 0 && required {
		return refuse("required")
	}
	if len(raw) > 0 && media.Schema != nil {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		value, err := decodeUniqueJSON(decoder, 0)
		if err != nil || !utf8.Valid(raw) || !validJSONEscapes(raw) {
			return refuse("invalid_json")
		}
		if _, err := decoder.Token(); err != io.EOF {
			return refuse("invalid_json")
		}
		if !boundedJSONNumbers(value) {
			return refuse("numeric_limit")
		}
		if media.Schema.Schema() == nil {
			return item, request, nil, ErrSchemaCompilation
		}
		valid, failures := requests.ValidateRequestSchema(&requests.ValidateRequestSchemaInput{
			Request: request, Schema: media.Schema.Schema(), Version: c.schemaVersion(),
			Options:      []validatorconfig.Option{validatorconfig.WithSchemaCache(nil), validatorconfig.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))},
			BodyRequired: required, DecodedValue: value, RawBody: raw, ValueDecoded: true,
		})
		if !valid {
			issues, err := validationIssues(failures)
			if err != nil || len(issues) == 0 {
				return item, request, nil, ErrSchemaCompilation
			}
			return item, request, issues, nil
		}
	}
	view, operation := operationValidationView(item, request.Method)
	operation.RequestBody = nil
	req := *request
	req.Body, req.GetBody, req.ContentLength = nil, nil, 0
	return view, &req, nil, nil
}

// Prefer the most specific matching range, regardless of document order.
// JSON suffixes identify decoding, not compatibility: application/json does
// not implicitly declare application/problem+json or vice versa.
func jsonBodyMedia(body *v3.RequestBody, contentType string) (*v3.MediaType, bool) {
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

// Go's JSON decoder replaces unpaired UTF-16 surrogates with U+FFFD. Reject
// those escapes instead of validating a value different from the supplied text.
// Syntax has already been checked by decodeUniqueJSON.
func validJSONEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) || raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil || unit >= 0xdc00 && unit <= 0xdfff {
			return false
		}
		i += 4
		if unit >= 0xd800 && unit <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
