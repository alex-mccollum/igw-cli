package apidocs

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const DefaultSpecFile = "openapi.json"

type Operation struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	OperationID string   `json:"operationId,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Deprecated  bool     `json:"deprecated,omitempty"`
}

type specDoc struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

type specOperation struct {
	OperationID string   `json:"operationId"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Deprecated  bool     `json:"deprecated"`
}

func LoadOperations(path string) ([]Operation, error) {
	b, err := os.ReadFile(path) //nolint:gosec // user-provided spec path
	if err != nil {
		return nil, fmt.Errorf("read spec file %q: %w", path, err)
	}

	var doc specDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse spec file %q: %w", path, err)
	}

	ops := make([]Operation, 0, 256)
	for apiPath, methods := range doc.Paths {
		for method, raw := range methods {
			normalized := strings.ToUpper(strings.TrimSpace(method))
			if !isHTTPMethod(normalized) {
				continue
			}

			var op specOperation
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, fmt.Errorf("parse operation %s %s from %q: %w", normalized, apiPath, path, err)
			}

			ops = append(ops, Operation{
				Method:      normalized,
				Path:        apiPath,
				OperationID: strings.TrimSpace(op.OperationID),
				Summary:     strings.TrimSpace(op.Summary),
				Description: strings.TrimSpace(op.Description),
				Tags:        copyStrings(op.Tags),
				Deprecated:  op.Deprecated,
			})
		}
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})

	return ops, nil
}

func FilterByMethod(ops []Operation, method string) []Operation {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return copyOperations(ops)
	}

	filtered := make([]Operation, 0, len(ops))
	for _, op := range ops {
		if op.Method == method {
			filtered = append(filtered, op)
		}
	}
	return filtered
}

func FilterByPathContains(ops []Operation, contains string) []Operation {
	contains = strings.ToLower(strings.TrimSpace(contains))
	if contains == "" {
		return copyOperations(ops)
	}

	filtered := make([]Operation, 0, len(ops))
	for _, op := range ops {
		if strings.Contains(strings.ToLower(op.Path), contains) {
			filtered = append(filtered, op)
		}
	}
	return filtered
}

func FilterByPath(ops []Operation, path string) []Operation {
	path = strings.TrimSpace(path)
	filtered := make([]Operation, 0, len(ops))
	for _, op := range ops {
		if op.Path == path {
			filtered = append(filtered, op)
		}
	}
	return filtered
}

func Search(ops []Operation, query string) []Operation {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return copyOperations(ops)
	}

	filtered := make([]Operation, 0, len(ops))
	for _, op := range ops {
		if operationContains(op, query) {
			filtered = append(filtered, op)
		}
	}
	return filtered
}

func operationContains(op Operation, query string) bool {
	if strings.Contains(strings.ToLower(op.Method), query) {
		return true
	}
	if strings.Contains(strings.ToLower(op.Path), query) {
		return true
	}
	if strings.Contains(strings.ToLower(op.OperationID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(op.Summary), query) {
		return true
	}
	if strings.Contains(strings.ToLower(op.Description), query) {
		return true
	}
	for _, tag := range op.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}

func copyOperations(ops []Operation) []Operation {
	if len(ops) == 0 {
		return nil
	}

	out := make([]Operation, len(ops))
	for i := range ops {
		out[i] = ops[i]
		out[i].Tags = copyStrings(ops[i].Tags)
	}
	return out
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
