package catalog

import (
	"mime"
	"strings"
	"unicode/utf8"
)

// Content parameters have one media representation, independent of the
// schema/style serialization strategy. Callers establish presence, uniqueness,
// and the effective text before decoding; this never changes the wire value.
func parameterContentValue(p *parameter, text string) (any, *schemaView, string) {
	if p.Schema != nil || p.Content == nil || len(p.Content) != 1 {
		return nil, nil, "unsupported_serialization"
	}
	if len(text) > MaxJSONBodyBytes {
		return nil, nil, "parameter_limit"
	}
	for media, content := range p.Content {
		if content == nil || content.Schema == nil || content.Schema.Schema() == nil {
			return nil, nil, "unsupported_serialization"
		}
		value, rule := decodeParameterContent(media, text)
		return value, content.Schema.Schema(), rule
	}
	return nil, nil, "unsupported_serialization"
}

func decodeParameterContent(media, text string) (any, string) {
	media, parameters, err := mime.ParseMediaType(media)
	if err != nil || strings.Contains(media, "*") {
		return nil, "unsupported_serialization"
	}
	charset := strings.ToLower(parameters["charset"])
	if charset != "" && charset != "utf-8" && charset != "us-ascii" {
		return nil, "unsupported_serialization"
	}
	if charset == "us-ascii" {
		for i := range len(text) {
			if text[i] >= utf8.RuneSelf {
				return nil, "invalid_text"
			}
		}
	}
	_, subtype, _ := strings.Cut(media, "/")
	if subtype == "json" || strings.HasSuffix(subtype, "+json") {
		return decodeExactJSON([]byte(text))
	}
	if media == "text/plain" {
		return parameterPrimitive("string", text)
	}
	return nil, "unsupported_serialization"
}
