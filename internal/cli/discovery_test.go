package cli

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestProgressiveSchemaAndJSONHelp(t *testing.T) {
	for _, args := range [][]string{{"schema"}, {"--help"}, {"schema", "api"}, {"api", "--help"}} {
		app, out, _ := testApp(t, nil)
		app.ReadConfig = func() (config.File, error) {
			t.Fatal("discovery read runtime configuration")
			return config.File{}, nil
		}
		if err := app.Run(context.Background(), append(args, "--json")); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(decodeResult(t, out).Data)
		var info commandInfo
		if json.Unmarshal(raw, &info) != nil || len(info.Flags) == 0 || len(info.Commands) == 0 {
			t.Fatal("selected command missing flags or children")
		}
		for _, child := range info.Commands {
			if len(child.Flags) > 0 || len(child.Commands) > 0 || child.Usage == "" || child.Description == "" {
				t.Fatal("default discovery recursed or omitted summary")
			}
		}
	}
	app, out, _ := testApp(t, nil)
	if err := app.Run(context.Background(), []string{"schema", "api", "--recursive", "--json"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(decodeResult(t, out).Data)
	var info commandInfo
	json.Unmarshal(raw, &info)
	for _, child := range info.Commands {
		if len(child.Flags) == 0 {
			t.Fatal("recursive discovery omitted flags")
		}
	}
}
