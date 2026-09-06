package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/buildinfo"
	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestVersionAliasesPreserveReleaseContract(t *testing.T) {
	version, commit, date := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit, buildinfo.Date = version, commit, date })
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "v1.0.0", "test-commit", "2026-09-06"
	for _, alias := range []string{"version", "--version", "-v"} {
		a, out, stderr := testApp(t, nil)
		a.ReadConfig = func() (config.File, error) { t.Fatal("version loaded configuration"); return config.File{}, nil }
		if err := a.Run(context.Background(), []string{alias}); err != nil {
			t.Fatal(err)
		}
		if out.String() != "igw version v1.0.0 (test-commit, 2026-09-06)\n" || stderr.Len() != 0 {
			t.Fatalf("version contract: %q", out.String())
		}
		out.Reset()
		if err := a.Run(context.Background(), []string{alias, "--json"}); err != nil {
			t.Fatal(err)
		}
		data := decodeResult(t, out).Data.(map[string]any)
		if data["version"] != "v1.0.0" || data["commit"] != "test-commit" || data["date"] != "2026-09-06" {
			t.Fatal("version metadata missing from JSON")
		}
	}
}

func TestSchemaCanInspectOneCommandWithoutConfiguration(t *testing.T) {
	for _, tc := range []struct {
		path []string
		name string
	}{{nil, "igw"}, {[]string{"resource", "update"}, "update"}, {[]string{"profile", "migrate"}, "migrate"}} {
		a, out, _ := testApp(t, nil)
		a.ReadConfig = func() (config.File, error) { t.Fatal("schema loaded configuration"); return config.File{}, nil }
		args := append([]string{"schema"}, tc.path...)
		args = append(args, "--json")
		if err := a.Run(context.Background(), args); err != nil {
			t.Fatal(err)
		}
		data := decodeResult(t, out).Data.(map[string]any)
		if data["name"] != tc.name || !strings.HasPrefix(data["usage"].(string), "igw") {
			t.Fatal("schema does not identify unified command")
		}
	}
}

func TestSupersededCommandsFailWithoutGatewayAccess(t *testing.T) {
	for _, args := range [][]string{{"rpc"}, {"call"}, {"config"}, {"tags"}, {"wait"}, {"restart"}, {"schema", "unknown"}, {"api", "unknown"}, {"profile", "unknown"}, {"diagnostics", "bundle", "unknown"}, {"completion", "unknown"}} {
		a, out, _ := testApp(t, nil)
		a.ReadConfig = func() (config.File, error) {
			t.Fatal("removed command loaded configuration")
			return config.File{}, nil
		}
		if err := a.Run(context.Background(), append(args, "--json")); err == nil {
			t.Fatal("unsupported command succeeded")
		}
		r := decodeResult(t, out)
		if r.OK || r.Error.Code != 2 {
			t.Fatal("unsupported command lost usage contract")
		}
	}
}

func TestCommandGroupHelpIsStructuredAndOffline(t *testing.T) {
	a, out, _ := testApp(t, nil)
	a.ReadConfig = func() (config.File, error) { t.Fatal("group help loaded configuration"); return config.File{}, nil }
	if err := a.Run(context.Background(), []string{"api", "--json"}); err != nil {
		t.Fatal(err)
	}
	if data := decodeResult(t, out).Data.(map[string]any); data["name"] != "api" {
		t.Fatal("group help did not use its command schema")
	}
}

func TestHelpRejectsUnknownCommandPaths(t *testing.T) {
	for _, path := range [][]string{{"unknown"}, {"api", "unknown"}, {"diagnostics", "bundle", "unknown"}} {
		for _, jsonOutput := range []bool{false, true} {
			a, out, stderr := testApp(t, nil)
			a.ReadConfig = func() (config.File, error) { t.Fatal("help loaded configuration"); return config.File{}, nil }
			args := append([]string{"help"}, path...)
			if jsonOutput {
				args = append(args, "--json")
			}
			if err := a.Run(context.Background(), args); err == nil {
				t.Fatalf("unknown help path succeeded: %v", path)
			}
			if jsonOutput {
				r := decodeResult(t, out)
				if r.OK || r.Error.Code != 2 || stderr.Len() != 0 {
					t.Fatal("unknown help path lost the JSON usage contract")
				}
			} else if out.Len() != 0 || stderr.Len() == 0 {
				t.Fatal("unknown help path must report its error on stderr")
			}
		}
	}
}

func TestHelpCommandRespectsSelectedCommand(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		a, out, stderr := testApp(t, nil)
		a.ReadConfig = func() (config.File, error) { t.Fatal("help loaded configuration"); return config.File{}, nil }
		args := []string{"help", "api"}
		if jsonOutput {
			args = append(args, "--json")
		}
		if err := a.Run(context.Background(), args); err != nil || stderr.Len() != 0 {
			t.Fatalf("valid help path failed: %v", err)
		}
		if jsonOutput {
			if decodeResult(t, out).Data.(map[string]any)["name"] != "api" {
				t.Fatal("help selected the wrong command schema")
			}
		} else if !strings.Contains(out.String(), "igw api") {
			t.Fatal("help selected the wrong command usage")
		}
	}
}
