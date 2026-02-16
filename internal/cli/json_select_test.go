package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestParseJSONFieldsCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "empty", raw: "", want: nil},
		{name: "single", raw: "response.status", want: []string{"response.status"}},
		{name: "trim and dedupe", raw: " ok , response.status , ok ", want: []string{"ok", "response.status"}},
		{name: "invalid empty selector", raw: "ok,,response.status", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseJSONFieldsCSV(tc.raw)
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
		compact: true,
		fields:  []string{"ok", "response.status"},
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
