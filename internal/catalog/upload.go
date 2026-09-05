package catalog

import (
	"mime"
	"strings"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// OpaqueUpload reports a declared body needing only transport checks: no schema
// or a recognized unconstrained binary form. Value-constrained schemas require
// a supported bounded decoder; a stream must not bypass those assertions.
func (c *Catalog) OpaqueUpload(key, contentType string) bool {
	op, err := c.Resolve(key)
	if err != nil {
		return false
	}
	c.validationMu.Lock()
	defer c.validationMu.Unlock()
	item := c.model.Model.Paths.PathItems.GetOrZero(op.Path)
	if item == nil {
		return false
	}
	var operation *v3.Operation
	switch op.Method {
	case "POST":
		operation = item.Post
	case "PUT":
		operation = item.Put
	case "PATCH":
		operation = item.Patch
	default:
		return false
	}
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Content == nil {
		return false
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.Contains(media, "/") || strings.Contains(media, "*") {
		return false
	}
	entry, ambiguous := requestBodyMedia(operation.RequestBody, media)
	if ambiguous || entry == nil {
		return false
	}
	encoding := bodyEncoding(media, entry)
	return encoding == "opaque" || encoding == "binary"
}
