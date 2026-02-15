package cli

import (
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestParseOptionalBoolFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "true lower", input: "true", want: "true"},
		{name: "true upper", input: "TRUE", want: "true"},
		{name: "false mixed", input: "FaLsE", want: "false"},
		{name: "trim whitespace", input: "  true  ", want: "true"},
		{name: "invalid", input: "yes", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseOptionalBoolFlag("test", tc.input)
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
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestParseRequiredEnumFlag(t *testing.T) {
	t.Parallel()

	allowed := []string{"DEBUG", "INFO", "WARN"}
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "exact", input: "DEBUG", want: "DEBUG"},
		{name: "case insensitive", input: "info", want: "INFO"},
		{name: "trimmed", input: "  warn  ", want: "WARN"},
		{name: "missing", input: "", wantErr: true},
		{name: "invalid", input: "TRACE", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseRequiredEnumFlag("level", tc.input, allowed)
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
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestParseRequiredEnumFlagErrorMentionsAllowedValues(t *testing.T) {
	t.Parallel()

	_, err := parseRequiredEnumFlag("type", "yaml", []string{"json", "xml"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "json, xml") {
		t.Fatalf("expected allowed values in error, got %q", err.Error())
	}
}
