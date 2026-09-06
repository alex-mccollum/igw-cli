package cli

import (
	"encoding/json"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

const maxMultipartManifestBytes = 1 << 20

const multipartManifestSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array","minItems":1,"maxItems":256,"items":{"type":"object","required":["name"],"additionalProperties":false,"properties":{"name":{"type":"string","minLength":1},"text":{"type":"string"},"file":{"type":"string","minLength":1},"filename":{"type":"string","minLength":1},"contentType":{"type":"string","minLength":1}},"oneOf":[{"required":["text"],"not":{"anyOf":[{"required":["file"]},{"required":["filename"]}]}},{"required":["file"],"not":{"required":["text"]}}]}}`

func (i *invocation) multipartParts(manifest string, fields, files []string) ([]artifact.MultipartPart, error) {
	if manifest != "" {
		raw, err := i.readInput(manifest, maxMultipartManifestBytes)
		if err != nil {
			return nil, err
		}
		return decodeMultipartParts(raw)
	}
	parts := make([]artifact.MultipartPart, 0, len(fields)+len(files))
	for _, pair := range fields {
		name, value, found := strings.Cut(pair, "=")
		if !found || name == "" {
			return nil, result.Usage("--form-field requires name=value; values are literal text")
		}
		parts = append(parts, artifact.MultipartPart{Name: name, Text: &value})
	}
	for _, pair := range files {
		name, path, found := strings.Cut(pair, "=")
		if !found || name == "" || path == "" {
			return nil, result.Usage("--form-file requires name=path to a regular file")
		}
		parts = append(parts, artifact.MultipartPart{Name: name, File: path})
	}
	return parts, nil
}

// Decode exact, case-sensitive field names and string values. A misspelled or
// duplicate file/text selector must never silently select different contents.
func decodeMultipartParts(raw []byte) ([]artifact.MultipartPart, error) {
	bad := result.Usage("--multipart requires an ordered JSON array of 1..256 parts with name, exactly one of text or file, and optional filename/contentType strings")
	if len(raw) > maxMultipartManifestBytes || jsonvalue.Validate(raw) != nil || !jsonvalue.ValidUnicode(raw) {
		return nil, bad
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 || len(rows) > artifact.MaxMultipartParts {
		return nil, bad
	}
	parts := make([]artifact.MultipartPart, len(rows))
	for n, row := range rows {
		_, text := row["text"]
		_, file := row["file"]
		if text == file {
			return nil, bad
		}
		for key, rawValue := range row {
			var value *string
			if json.Unmarshal(rawValue, &value) != nil || value == nil {
				return nil, bad
			}
			switch key {
			case "name":
				parts[n].Name = *value
			case "text":
				parts[n].Text = value
			case "file":
				if *value == "" {
					return nil, bad
				}
				parts[n].File = *value
			case "filename":
				if text || *value == "" {
					return nil, bad
				}
				parts[n].Filename = *value
			case "contentType":
				if *value == "" {
					return nil, bad
				}
				parts[n].ContentType = *value
			default:
				return nil, bad
			}
		}
		if parts[n].Name == "" {
			return nil, bad
		}
	}
	return parts, nil
}
