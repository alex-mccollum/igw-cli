package catalog

import (
	"mime"
	"strings"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// OpaqueUpload reports a declared media type whose body has no schema. Such a
// contract can validate media type, presence, and parameters without loading
// the uploaded bytes. Schema-bearing uploads must use the bounded body path.
func (c *Catalog) OpaqueUpload(key, contentType string) bool {
	op, err := c.Resolve(key)
	if err != nil {
		return false
	}
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
	if err != nil {
		return false
	}
	entry := operation.RequestBody.Content.GetOrZero(strings.ToLower(media))
	return entry != nil && entry.Schema == nil
}
