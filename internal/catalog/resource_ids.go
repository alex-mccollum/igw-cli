package catalog

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// normalizeResourceIDs handles the generator's duplicated, reference-free
// settings variants in config and backupConfig. Identity keywords alone do not
// constrain instances. Removing them is safe only when they are unused for
// reference resolution or dialect selection. Never weaken assertions (including
// oneOf) to make an otherwise unusable vendor schema accept a request.
func normalizeResourceIDs(op map[string]any, key, path, pointer string) []Adjustment {
	resource, ok := strings.CutPrefix(path, "/data/api/v1/resources/")
	if !ok || len(strings.Split(resource, "/")) != 2 || strings.ContainsAny(resource, "{}?#") {
		return nil
	}
	body := object(op["requestBody"])
	media := object(object(body["content"])["application/json"])
	schema := object(media["schema"])
	items := object(schema["items"])
	if schema["type"] != "array" || items["type"] != "object" {
		return nil
	}
	properties := object(items["properties"])
	variants := func(name string) []any {
		configProperties := object(object(properties[name])["properties"])
		settings := object(configProperties["settings"])
		values, _ := settings["oneOf"].([]any)
		return values
	}
	primary, backup := variants("config"), variants("backupConfig")
	if len(primary) == 0 || len(primary) != len(backup) {
		return nil
	}
	counts := make(map[string]int)
	if !resourceIDScope(schema, false, counts) || len(counts) != len(primary) {
		return nil
	}
	type pair struct {
		primary, backup           map[string]any
		primaryIndex, backupIndex int
	}
	pairs := make([]pair, 0, len(primary))
	seen := make(map[string]bool)
	for index, raw := range primary {
		variant := object(raw)
		id, _ := variant["$id"].(string)
		title, _ := variant["title"].(string)
		if title == "" || id != resource+"/"+title || variant["type"] != "object" || counts[id] != 2 || seen[id] || !resourceIDScope(variant, true, nil) {
			return nil
		}
		seen[id] = true
		matched := false
		for otherIndex, otherRaw := range backup {
			other := object(otherRaw)
			if other["$id"] != id {
				continue
			}
			a, _ := json.Marshal(variant)
			b, _ := json.Marshal(other)
			if !bytes.Equal(a, b) {
				return nil
			}
			pairs = append(pairs, pair{variant, other, index, otherIndex})
			matched = true
			break
		}
		if !matched {
			return nil
		}
	}
	// The whole operation qualifies before touching the private model. A single
	// unresolved variant must remain a schema-availability error.
	var adjustments []Adjustment
	base := pointer + "/requestBody/content/application~1json/schema/items/properties/"
	for _, pair := range pairs {
		delete(pair.primary, "$id")
		delete(pair.backup, "$id")
		for _, part := range []struct {
			name  string
			index int
		}{{"config", pair.primaryIndex}, {"backupConfig", pair.backupIndex}} {
			adjustments = append(adjustments, Adjustment{Operation: key, Pointer: base + part.name + "/properties/settings/oneOf/" + strconv.Itoa(part.index) + "/$id", Rule: "resource-unused-duplicate-id"})
		}
	}
	return adjustments
}

func object(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

// The whole request may reference components outside the settings variants.
// Each variant itself must be reference-free, without anchors/dialects or any
// other scope keyword. Scanning opaque instance data too is conservative: it
// may refuse an adapter but cannot silently overlook a reference dependency.
func resourceIDScope(value any, variant bool, ids map[string]int) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch {
			case key == "$id":
				id, ok := child.(string)
				if !ok {
					return false
				}
				if ids != nil {
					ids[id]++
				}
			case key == "$ref" && !variant:
				// References elsewhere are unchanged; catalog parsing separately
				// requires that they resolve within the original document.
			case strings.HasPrefix(key, "$") || key == "id" || key == "discriminator" || key == "jsonSchemaDialect":
				return false
			}
			if !resourceIDScope(child, variant, ids) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !resourceIDScope(child, variant, ids) {
				return false
			}
		}
	}
	return true
}
