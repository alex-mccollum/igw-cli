package catalog

import (
	"mime"
	"sort"
	"strings"
)

// BodyInput describes the CLI's support alongside the unchanged vendor schema.
// A media range needs an actual Content-Type before decoder support is known.
type BodyInput struct {
	MediaType      string `json:"mediaType"`
	Required       bool   `json:"required"`
	SchemaDeclared bool   `json:"schemaDeclared"`
	Encoding       string `json:"encoding"`
	Validation     string `json:"validation"`
	Streaming      string `json:"streaming"`
}

func (c *Catalog) bodyInputs(op Operation) []BodyInput {
	c.validationMu.Lock()
	defer c.validationMu.Unlock()
	item := c.model.Model.Paths.PathItems.GetOrZero(op.Path)
	definition := item.GetOperations().GetOrZero(strings.ToLower(op.Method))
	body := definition.RequestBody
	if body == nil || body.Content == nil {
		return nil
	}
	names := make(map[string]int)
	for name := range body.Content.FromOldest() {
		names[strings.ToLower(name)]++
	}
	inputs := make([]BodyInput, 0, body.Content.Len())
	for name, media := range body.Content.FromOldest() {
		input := BodyInput{MediaType: name, Required: body.Required != nil && *body.Required,
			SchemaDeclared: media.Schema != nil, Encoding: "unsupported", Validation: "unsupported", Streaming: "unsupported"}
		normalized, parameters, err := mime.ParseMediaType(name)
		top, sub, slash := strings.Cut(normalized, "/")
		valid := err == nil && len(parameters) == 0 && slash && names[strings.ToLower(name)] == 1
		valid = valid && ((top == "*" && sub == "*") ||
			(!strings.Contains(top, "*") && (sub == "*" || !strings.Contains(sub, "*"))))
		if valid && sub == "*" && media.Schema != nil {
			input.Encoding, input.Validation = "selected_media", "selected_media"
			if op.Method == "POST" || op.Method == "PUT" || op.Method == "PATCH" {
				input.Streaming = "selected_media"
			}
		} else if valid {
			input.Encoding = bodyEncoding(normalized, media)
			switch input.Encoding {
			case "opaque", "binary":
				input.Validation = ValidationTransport
				if op.Method == "POST" || op.Method == "PUT" || op.Method == "PATCH" {
					input.Streaming = "supported"
				}
			case "json", "utf8", "urlencoded":
				input.Validation = ValidationSchema
			}
		}
		inputs = append(inputs, input)
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].MediaType < inputs[j].MediaType })
	return inputs
}
