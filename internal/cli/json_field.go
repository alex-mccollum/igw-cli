package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func extractJSONField(payload any, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("field path is empty")
	}

	var root any
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode payload: %w", err)
	}
	if err := json.Unmarshal(b, &root); err != nil {
		return "", fmt.Errorf("decode payload: %w", err)
	}

	current := root
	segments := strings.Split(path, ".")
	for _, segment := range segments {
		token := strings.TrimSpace(segment)
		if token == "" {
			return "", fmt.Errorf("invalid path segment in %q", path)
		}

		switch node := current.(type) {
		case map[string]any:
			next, ok := node[token]
			if !ok {
				return "", fmt.Errorf("key %q not found", token)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil {
				return "", fmt.Errorf("expected array index, got %q", token)
			}
			if index < 0 || index >= len(node) {
				return "", fmt.Errorf("array index %d out of range", index)
			}
			current = node[index]
		default:
			return "", fmt.Errorf("cannot descend through %T at %q", current, token)
		}
	}

	return formatExtractedJSONValue(current)
}

func formatExtractedJSONValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("encode value: %w", err)
		}
		return string(b), nil
	}
}
