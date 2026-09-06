package cli

import (
	"context"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"testing"
)

func TestOperationalUsageIsCheckedBeforeConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"backup", "export"}, {"backup", "export", "--out", "backup.gwbk", "--max-bytes", "0"},
		{"logs", "list", "--since", "tomorrow"}, {"logs", "list", "--min-level", "guess"}, {"logs", "list", "--limit", "0"},
		{"diagnostics", "bundle", "collect", "--out", "bundle.zip"}, {"diagnostics", "bundle", "collect", "--out", "bundle.zip", "--dry-run", "--interval", "0s"},
		{"diagnostics", "bundle", "download"}, {"diagnostics", "bundle", "status", "--offline"},
	} {
		app, out, _ := testApp(t, nil)
		app.ReadConfig = func() (config.File, error) {
			t.Fatal("invalid command loaded configuration")
			return config.File{}, nil
		}
		err := app.Run(context.Background(), append(args, "--json"))
		got := decodeResult(t, out)
		if err == nil || got.OK || got.Error.Code != 2 {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}
