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

func TestInferTagImportType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "json", path: "tags.json", want: "json"},
		{name: "xml upper extension", path: "TAGS.XML", want: "xml"},
		{name: "csv", path: "/tmp/tags.csv", want: "csv"},
		{name: "unknown extension", path: "tags.txt", want: ""},
		{name: "no extension", path: "tags", want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := inferTagImportType(tc.path)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestChooseDefaultOutPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		explicit    string
		defaultName string
		interactive bool
		want        string
	}{
		{
			name:        "explicit wins",
			explicit:    "custom.zip",
			defaultName: "gateway-logs.zip",
			interactive: true,
			want:        "custom.zip",
		},
		{
			name:        "interactive uses default",
			explicit:    "",
			defaultName: "gateway-logs.zip",
			interactive: true,
			want:        "gateway-logs.zip",
		},
		{
			name:        "non interactive leaves stdout",
			explicit:    "",
			defaultName: "gateway-logs.zip",
			interactive: false,
			want:        "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chooseDefaultOutPath(tc.explicit, tc.defaultName, tc.interactive)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
