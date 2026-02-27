package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExtractJSONPathRawScalar(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"response": map[string]any{
			"status": 200,
		},
	}

	got, err := extractJSONPathRaw(payload, "response.status")
	if err != nil {
		t.Fatalf("extract path: %v", err)
	}
	if got != "200" {
		t.Fatalf("unexpected extracted value %q", got)
	}
}

func TestExtractJSONPathRawArrayPath(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"checks": []any{
			map[string]any{"name": "gateway_url"},
		},
	}

	got, err := extractJSONPathRaw(payload, "checks.0.name")
	if err != nil {
		t.Fatalf("extract path: %v", err)
	}
	if got != "gateway_url" {
		t.Fatalf("unexpected extracted value %q", got)
	}
}

func TestExtractJSONPathRawObjectValue(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"details": map[string]any{
			"status": 403,
			"hint":   "forbidden",
		},
	}

	got, err := extractJSONPathRaw(payload, "details")
	if err != nil {
		t.Fatalf("extract path: %v", err)
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

func TestExtractJSONPathRawInvalidPath(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"response": map[string]any{
			"status": 200,
		},
		"checks": []any{"x"},
	}

	_, err := extractJSONPathRaw(payload, "response..status")
	if err == nil || !strings.Contains(err.Error(), "invalid path segment") {
		t.Fatalf("expected invalid path segment error, got %v", err)
	}

	_, err = extractJSONPathRaw(payload, "response.code")
	if err == nil || !strings.Contains(err.Error(), "key \"code\" not found") {
		t.Fatalf("expected key missing error, got %v", err)
	}

	_, err = extractJSONPathRaw(payload, "checks.foo")
	if err == nil || !strings.Contains(err.Error(), "expected array index") {
		t.Fatalf("expected array index error, got %v", err)
	}

	_, err = extractJSONPathRaw(payload, " ")
	if err == nil || !strings.Contains(err.Error(), "select path is empty") {
		t.Fatalf("expected empty path error, got %v", err)
	}
}

func TestNormalizeJSONPayloadNativeTypes(t *testing.T) {
	t.Parallel()

	t.Run("native map and scalar values", func(t *testing.T) {
		t.Parallel()

		inputs := []any{
			map[string]any{"a": "b"},
			[]any{"x", 1},
			"value",
			true,
			float64(3.14),
			nil,
		}

		for _, input := range inputs {
			got, err := normalizeJSONPayload(input)
			if err != nil {
				t.Fatalf("normalize failed for %T: %v", input, err)
			}
			if !reflect.DeepEqual(got, input) {
				t.Fatalf("expected passthrough for %T; got %#v", input, got)
			}
		}
	})

	t.Run("JSON bytes decode", func(t *testing.T) {
		t.Parallel()

		got, err := normalizeJSONPayload([]byte(`{"ok":true}`))
		if err != nil {
			t.Fatalf("decode bytes failed: %v", err)
		}

		decoded, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", got)
		}
		if decoded["ok"] != true {
			t.Fatalf("unexpected decoded value %#v", decoded)
		}
	})

	t.Run("json.RawMessage decode", func(t *testing.T) {
		t.Parallel()

		got, err := normalizeJSONPayload(json.RawMessage(`[1,2,3]`))
		if err != nil {
			t.Fatalf("decode raw message failed: %v", err)
		}

		decoded, ok := got.([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", got)
		}
		if len(decoded) != 3 {
			t.Fatalf("unexpected length %d", len(decoded))
		}
	})
}

func TestNormalizeJSONPayloadFallbackRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("struct fallback", func(t *testing.T) {
		t.Parallel()

		type fixture struct {
			ID   string `json:"id"`
			Flag bool   `json:"flag"`
		}

		got, err := normalizeJSONPayload(fixture{ID: "x", Flag: true})
		if err != nil {
			t.Fatalf("normalize fallback failed: %v", err)
		}

		decoded, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", got)
		}
		if decoded["id"] != "x" || decoded["flag"] != true {
			t.Fatalf("unexpected fallback decode %#v", decoded)
		}
	})
}
