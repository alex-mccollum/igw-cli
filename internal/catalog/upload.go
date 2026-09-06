package catalog

import (
	"mime"
	"strings"
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
	if op.Method != "POST" && op.Method != "PUT" && op.Method != "PATCH" {
		return false
	}
	item, err := c.requestContract(op)
	if err != nil {
		return false
	}
	operation := item.Operation
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
