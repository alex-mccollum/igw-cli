package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestNormalizeJSONSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     []string
		want    []string
		wantErr bool
	}{
		{name: "empty", raw: nil, want: nil},
		{name: "single", raw: []string{"response.status"}, want: []string{"response.status"}},
		{name: "trim and dedupe", raw: []string{" ok ", "response.status", "ok"}, want: []string{"ok", "response.status"}},
		{name: "invalid empty selector", raw: []string{"ok", ""}, wantErr: true},
		{name: "comma not supported", raw: []string{"ok,response.status"}, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeJSONSelectors(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if code := igwerr.ExitCode(err); code != 2 {
					t.Fatalf("unexpected exit code %d", code)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("unexpected length: got %d want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d]=%q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNewJSONSelectOptionsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		json      bool
		compact   bool
		raw       bool
		selectors []string
		wantErr   string
	}{
		{name: "compact without json", compact: true, wantErr: "required: --json when using --compact"},
		{name: "raw without json", raw: true, selectors: []string{"ok"}, wantErr: "required: --json when using --raw"},
		{name: "select without json", selectors: []string{"ok"}, wantErr: "required: --json when using --select"},
		{name: "raw needs one selector none", json: true, raw: true, wantErr: "required: exactly one --select when using --raw"},
		{name: "raw needs one selector many", json: true, raw: true, selectors: []string{"ok", "status"}, wantErr: "required: exactly one --select when using --raw"},
		{name: "raw and compact invalid", json: true, raw: true, compact: true, selectors: []string{"ok"}, wantErr: "cannot use --raw with --compact"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newJSONSelectOptions(tc.json, tc.compact, tc.raw, tc.selectors)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPrintJSONSelectionFieldsCompact(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"ok": true,
		"response": map[string]any{
			"status": 200,
		},
	}

	var out bytes.Buffer
	err := printJSONSelection(&out, payload, jsonSelectOptions{
		compact:   true,
		selectors: []string{"ok", "response.status"},
	})
	if err != nil {
		t.Fatalf("print selection failed: %v", err)
	}

	output := out.String()
	if !json.Valid([]byte(output)) {
		t.Fatalf("expected valid json output, got %q", output)
	}
	if strings.Contains(output, "\n  ") {
		t.Fatalf("expected compact output, got %q", output)
	}
}

func TestPrintJSONSelectionRaw(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"response": map[string]any{
			"status": 200,
		},
	}

	var out bytes.Buffer
	err := printJSONSelection(&out, payload, jsonSelectOptions{
		raw:       true,
		selectors: []string{"response.status"},
	})
	if err != nil {
		t.Fatalf("print selection failed: %v", err)
	}
	if out.String() != "200\n" {
		t.Fatalf("unexpected raw output %q", out.String())
	}
}
