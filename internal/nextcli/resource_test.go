package nextcli

import (
	"context"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestResourceInputErrorsDoNotLoadGatewayConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"resource", "create", "ignition/schedule", "test", "--body", `{"enabled":true}`},
		{"resource", "update", "ignition/schedule", "test", "--body", `{"enabled":true}`, "--yes"},
		{"resource", "delete", "ignition/schedule", "test", "--yes"},
		{"resource", "create", "ignition/schedule", "test", "--body", `{"signature":"hidden"}`, "--yes"},
		{"resource", "update", "ignition/schedule", "test", "--body", `{}`, "--dry-run", "--offline"},
		{"resource", "list", "ignition/schedule", "--limit", "0"},
		{"resource", "get", "../schedule", "test"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			app, out, _ := testApp(t, nil)
			app.ReadConfig = func() (config.File, error) { t.Fatal("invalid input loaded configuration"); return config.File{}, nil }
			err := app.Run(context.Background(), append(args, "--json"))
			got := decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || got.OK || got.Error.Code != 2 {
				t.Fatalf("incorrect input result: %+v", got)
			}
		})
	}
}

func TestResourceCommandSchemaIncludesReviewPrecondition(t *testing.T) {
	app, out, _ := testApp(t, nil)
	app.ReadConfig = func() (config.File, error) { t.Fatal("help loaded configuration"); return config.File{}, nil }
	if err := app.Run(context.Background(), []string{"resource", "update", "--help", "--json"}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{"if-signature", "dry-run", "body", "collection", "yes"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("schema omitted %s", expected)
		}
	}
	decodeResult(t, out)
}
