package catalog

import (
	"bytes"
	"mime"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

const MaxFormFields = 4096

// decodeFormBody interprets the actual wire bytes. Only directly declared
// object properties have a known field binding; composed/dynamic properties
// and undefined encodings are refused rather than guessed. The caller validates
// the complete decoded object with the request-purpose schema compiler.
func decodeFormBody(raw []byte, media *v3.MediaType, charset string) (any, string) {
	if len(raw) > MaxJSONBodyBytes || bytes.Count(raw, []byte{'&'}) >= MaxFormFields {
		return nil, "form_limit"
	}
	charset = strings.ToLower(charset)
	if charset != "" && charset != "utf-8" && charset != "us-ascii" {
		return nil, "unsupported_serialization"
	}
	schema := media.Schema.Schema()
	if schema == nil || len(schema.Type) != 1 || schema.Type[0] != "object" {
		return nil, "unsupported_serialization"
	}
	fields, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, "invalid_form_encoding"
	}
	value := make(map[string]any, len(fields))
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		values := fields[name]
		if name == "" || !utf8.ValidString(name) {
			return nil, "invalid_form_encoding"
		}
		for _, text := range append([]string{name}, values...) {
			if !utf8.ValidString(text) {
				return nil, "invalid_utf8"
			}
			if charset == "us-ascii" {
				for _, b := range []byte(text) {
					if b >= utf8.RuneSelf {
						return nil, "invalid_text"
					}
				}
			}
		}
		if schema.Properties == nil {
			return nil, "unsupported_serialization"
		}
		property := schema.Properties.GetOrZero(name)
		if property == nil || property.Schema() == nil {
			return nil, "unsupported_serialization"
		}
		var encoding *v3.Encoding
		if media.Encoding != nil {
			encoding = media.Encoding.GetOrZero(name)
		}
		decoded, rule := decodeFormProperty(property.Schema(), encoding, values)
		if rule != "" {
			return nil, rule
		}
		value[name] = decoded
	}
	return value, ""
}

func decodeFormProperty(schema *base.Schema, encoding *v3.Encoding, values []string) (any, string) {
	kind := querySchemaKind(schema)
	contentType := ""
	if encoding != nil {
		contentType = encoding.ContentType
		// Explicit false is significant: it selects RFC6570 serialization
		// just as explicit style/explode do. Read presence from the source node.
		reservedSet := encoding.GoLow() != nil && !encoding.GoLow().AllowReserved.IsEmpty()
		if encoding.Style != "" || encoding.Explode != nil || reservedSet || encoding.AllowReserved {
			if (encoding.Style != "" && encoding.Style != "form") || encoding.AllowReserved {
				return nil, "unsupported_serialization"
			}
			if kind == "array" {
				if encoding.Explode != nil && !*encoding.Explode {
					return nil, "unsupported_serialization"
				}
				members := make([]any, 0, len(values))
				for _, text := range values {
					member, rule := parameterPrimitive(querySchemaKind(schema.Items.A.Schema()), text)
					if rule != "" {
						return nil, rule
					}
					members = append(members, member)
				}
				return members, ""
			}
			if len(values) != 1 {
				return nil, "duplicate_form_field"
			}
			return parameterPrimitive(kind, values[0])
		}
	}
	if len(values) != 1 {
		return nil, "duplicate_form_field"
	}
	if contentType == "" {
		switch {
		case schema.ContentEncoding != "" || schema.Format == "binary" || schema.Format == "byte":
			return nil, "unsupported_serialization"
		case kind == "string" || kind == "integer" || kind == "number" || kind == "boolean":
			return parameterPrimitive(kind, values[0])
		case len(schema.Type) == 1 && schema.Type[0] == "object":
			contentType = "application/json"
		default:
			// In particular, do not guess an implicit array's wire spelling.
			return nil, "unsupported_serialization"
		}
	}
	decoded, rule := decodeParameterContent(contentType, values[0])
	if rule != "" {
		return nil, rule
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "text/plain" {
		return parameterPrimitive(kind, decoded.(string))
	}
	return decoded, ""
}
