package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func extractJSONField(payload any, path string) (string, error) {
	value, err := extractJSONValue(payload, path)
	if err != nil {
		return "", err
	}
	return formatExtractedJSONValue(value)
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

func selectJSONFields(payload any, selectors []string) (map[string]any, error) {
	root, err := normalizeJSONPayload(payload)
	if err != nil {
		return nil, err
	}

	out := make(map[string]any, len(selectors))
	for _, selector := range selectors {
		value, extractErr := extractJSONValueFromRoot(root, selector)
		if extractErr != nil {
			return nil, &igwerr.UsageError{
				Msg: fmt.Sprintf("invalid --fields selector %q: %v", strings.TrimSpace(selector), extractErr),
			}
		}
		out[selector] = value
	}

	return out, nil
}

func extractJSONValue(payload any, path string) (any, error) {
	root, err := normalizeJSONPayload(payload)
	if err != nil {
		return nil, err
	}
	return extractJSONValueFromRoot(root, path)
}

func normalizeJSONPayload(payload any) (any, error) {
	var root any

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	return root, nil
}

func extractJSONValueFromRoot(root any, path string) (any, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("field path is empty")
	}

	current := root
	segments := strings.Split(path, ".")
	for _, segment := range segments {
		token := strings.TrimSpace(segment)
		if token == "" {
			return nil, fmt.Errorf("invalid path segment in %q", path)
		}

		switch node := current.(type) {
		case map[string]any:
			next, ok := node[token]
			if !ok {
				return nil, fmt.Errorf("key %q not found", token)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("expected array index, got %q", token)
			}
			if index < 0 || index >= len(node) {
				return nil, fmt.Errorf("array index %d out of range", index)
			}
			current = node[index]
		default:
			return nil, fmt.Errorf("cannot descend through %T at %q", current, token)
		}
	}

	return current, nil
}
