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

type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Stats struct {
	Total        int     `json:"total"`
	Methods      []Count `json:"methods"`
	Tags         []Count `json:"tags"`
	PathPrefixes []Count `json:"pathPrefixes"`
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

func FilterByOperationID(ops []Operation, operationID string) []Operation {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil
	}

	exact := make([]Operation, 0, 2)
	for _, op := range ops {
		if op.OperationID == operationID {
			exact = append(exact, op)
		}
	}
	if len(exact) > 0 {
		return exact
	}

	lowerOperationID := strings.ToLower(operationID)
	fallback := make([]Operation, 0, 2)
	for _, op := range ops {
		if strings.ToLower(op.OperationID) == lowerOperationID {
			fallback = append(fallback, op)
		}
	}

	return fallback
}

func UniqueTags(ops []Operation) []string {
	if len(ops) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(ops))
	for _, op := range ops {
		for _, tag := range op.Tags {
			trimmed := strings.TrimSpace(tag)
			if trimmed == "" {
				continue
			}
			seen[trimmed] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}

	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func BuildStats(ops []Operation) Stats {
	return BuildStatsWithPrefixDepth(ops, 0)
}

func BuildStatsWithPrefixDepth(ops []Operation, prefixDepth int) Stats {
	methods := make(map[string]int, len(ops))
	tags := make(map[string]int, len(ops))
	pathPrefixes := make(map[string]int, len(ops))

	for _, op := range ops {
		methods[op.Method]++
		pathPrefixes[pathPrefix(op.Path, prefixDepth)]++

		if len(op.Tags) == 0 {
			tags["_untagged"]++
			continue
		}

		seenTags := make(map[string]struct{}, len(op.Tags))
		for _, tag := range op.Tags {
			trimmed := strings.TrimSpace(tag)
			if trimmed == "" {
				continue
			}
			if _, ok := seenTags[trimmed]; ok {
				continue
			}
			seenTags[trimmed] = struct{}{}
			tags[trimmed]++
		}
		if len(seenTags) == 0 {
			tags["_untagged"]++
		}
	}

	return Stats{
		Total:        len(ops),
		Methods:      sortedCounts(methods),
		Tags:         sortedCounts(tags),
		PathPrefixes: sortedCounts(pathPrefixes),
	}
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

func pathPrefix(apiPath string, depth int) string {
	apiPath = strings.TrimSpace(apiPath)
	if apiPath == "" {
		return "/"
	}

	parts := strings.Split(strings.Trim(apiPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "/"
	}

	if depth > 0 {
		if depth > len(parts) {
			depth = len(parts)
		}
		return "/" + strings.Join(parts[:depth], "/")
	}

	if len(parts) >= 4 && parts[0] == "data" && parts[1] == "api" && strings.HasPrefix(parts[2], "v") {
		return "/" + strings.Join(parts[:4], "/")
	}
	if len(parts) >= 2 {
		return "/" + strings.Join(parts[:2], "/")
	}
	return "/" + parts[0]
}

func sortedCounts(in map[string]int) []Count {
	if len(in) == 0 {
		return nil
	}

	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]Count, 0, len(keys))
	for _, k := range keys {
		out = append(out, Count{Name: k, Count: in[k]})
	}
	return out
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
