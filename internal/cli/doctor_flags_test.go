package cli

import (
	"strings"
	"testing"
)

func TestDoctorFlagContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		args         []string
		expectSubstr string
	}{
		{
			name: "select requires json",
			args: []string{
				"doctor",
				"--gateway-url", "http://127.0.0.1:8088",
				"--api-key", "secret",
				"--select", "checks.0.name",
			},
			expectSubstr: "required: --json when using --select",
		},
		{
			name: "raw requires json",
			args: []string{
				"doctor",
				"--gateway-url", "http://127.0.0.1:8088",
				"--api-key", "secret",
				"--select", "checks.0.name",
				"--raw",
			},
			expectSubstr: "required: --json when using --raw",
		},
		{
			name: "compact requires json",
			args: []string{
				"doctor",
				"--gateway-url", "http://127.0.0.1:8088",
				"--api-key", "secret",
				"--compact",
			},
			expectSubstr: "required: --json when using --compact",
		},
		{
			name: "raw requires exactly one select",
			args: []string{
				"doctor",
				"--gateway-url", "http://127.0.0.1:8088",
				"--api-key", "secret",
				"--json",
				"--select", "checks.0.name",
				"--select", "ok",
				"--raw",
			},
			expectSubstr: "required: exactly one --select when using --raw",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := newDoctorTestCLI(nil, nil).Execute(tc.args)
			requireDoctorUsageError(t, err)
			if !strings.Contains(err.Error(), tc.expectSubstr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
