package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestHelpPrecedesValueTakingGlobalFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--help", "--timeout", "90s"},
		{"-h", "--profile", "unconfigured", "--timeout", "90s"},
		{"profile", "--help", "--timeout", "90s"},
		{"logs", "list", "--help", "--timeout", "90s"},
	} {
		for _, machine := range []bool{false, true} {
			t.Run(strings.Join(args, " ")+map[bool]string{false: " human", true: " JSON"}[machine], func(t *testing.T) {
				app, out, stderr := testApp(t, nil)
				app.ReadConfig = func() (config.File, error) { t.Fatal("help loaded runtime configuration"); return config.File{}, nil }
				command := append([]string(nil), args...)
				if machine {
					command = append(command, "--json")
				}
				if err := app.Run(context.Background(), command); err != nil {
					t.Fatalf("help treated a flag value as a command: %v", err)
				}
				if stderr.Len() != 0 {
					t.Fatal("help emitted an error")
				}
				if machine {
					if !decodeResult(t, out).OK {
						t.Fatal("help JSON failed")
					}
				} else if !strings.Contains(out.String(), "Usage:") {
					t.Fatal("help output missing")
				}
			})
		}
	}
}
