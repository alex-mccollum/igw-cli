package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractJSONFieldScalar(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"response": map[string]any{
			"status": 200,
		},
	}

	got, err := extractJSONField(payload, "response.status")
	if err != nil {
		t.Fatalf("extract field: %v", err)
	}
	if got != "200" {
		t.Fatalf("unexpected extracted value %q", got)
	}
}

func TestExtractJSONFieldArrayPath(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"checks": []any{
			map[string]any{"name": "gateway_url"},
		},
	}

	got, err := extractJSONField(payload, "checks.0.name")
	if err != nil {
		t.Fatalf("extract field: %v", err)
	}
	if got != "gateway_url" {
		t.Fatalf("unexpected extracted value %q", got)
	}
}

func TestExtractJSONFieldObjectValue(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"details": map[string]any{
			"status": 403,
			"hint":   "forbidden",
		},
	}

	got, err := extractJSONField(payload, "details")
	if err != nil {
		t.Fatalf("extract field: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("decode extracted json: %v", err)
	}
	if int(decoded["status"].(float64)) != 403 {
		t.Fatalf("unexpected status %#v", decoded["status"])
	}
	if decoded["hint"] != "forbidden" {
		t.Fatalf("unexpected hint %#v", decoded["hint"])
	}
}

func TestExtractJSONFieldInvalidPath(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"response": map[string]any{
			"status": 200,
		},
		"checks": []any{"x"},
	}

	_, err := extractJSONField(payload, "response..status")
	if err == nil || !strings.Contains(err.Error(), "invalid path segment") {
		t.Fatalf("expected invalid path segment error, got %v", err)
	}

	_, err = extractJSONField(payload, "response.code")
	if err == nil || !strings.Contains(err.Error(), "key \"code\" not found") {
		t.Fatalf("expected key missing error, got %v", err)
	}

	_, err = extractJSONField(payload, "checks.foo")
	if err == nil || !strings.Contains(err.Error(), "expected array index") {
		t.Fatalf("expected array index error, got %v", err)
	}

	_, err = extractJSONField(payload, " ")
	if err == nil || !strings.Contains(err.Error(), "field path is empty") {
		t.Fatalf("expected empty path error, got %v", err)
	}
}
