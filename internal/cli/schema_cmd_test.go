package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestSchemaCommandRoot(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"schema"}); err != nil {
		t.Fatalf("schema failed: %v", err)
	}

	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		Command       struct {
			Name        string `json:"name"`
			Subcommands []struct {
				Name string `json:"name"`
			} `json:"subcommands"`
		} `json:"command"`
		GlobalFlags []string       `json:"globalFlags"`
		ExitCodes   map[string]int `json:"exitCodes"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema json: %v", err)
	}
	if payload.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %d", payload.SchemaVersion)
	}
	if payload.Command.Name != "igw" {
		t.Fatalf("unexpected root command: %q", payload.Command.Name)
	}
	if len(payload.Command.Subcommands) == 0 {
		t.Fatalf("expected root subcommands in schema payload")
	}
	if len(payload.GlobalFlags) == 0 {
		t.Fatalf("expected global flags in schema payload")
	}
	if payload.ExitCodes["usage"] != 2 {
		t.Fatalf("expected usage exit code in schema payload: %#v", payload.ExitCodes)
	}
}

func TestSchemaCommandPathLookup(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"schema", "config", "profile"}); err != nil {
		t.Fatalf("schema command path failed: %v", err)
	}

	var payload struct {
		Command struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Subcommands []struct {
				Name string `json:"name"`
			} `json:"subcommands"`
		} `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema json: %v", err)
	}
	if payload.Command.Name != "profile" {
		t.Fatalf("expected command name profile, got %q", payload.Command.Name)
	}
	if payload.Command.Path != "igw config profile" {
		t.Fatalf("unexpected command path %q", payload.Command.Path)
	}

	names := map[string]bool{}
	for _, sub := range payload.Command.Subcommands {
		names[sub.Name] = true
	}
	for _, expected := range []string{"add", "list", "use"} {
		if !names[expected] {
			t.Fatalf("expected nested subcommand %q in schema payload", expected)
		}
	}
}

func TestSchemaCommandPathLookupFlag(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"schema", "--command", "config profile"}); err != nil {
		t.Fatalf("schema --command failed: %v", err)
	}

	var payload struct {
		Command struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema json: %v", err)
	}
	if payload.Command.Name != "profile" || payload.Command.Path != "igw config profile" {
		t.Fatalf("unexpected schema --command payload: %#v", payload.Command)
	}
}

func TestSchemaCommandRejectsMixedPathSelectors(t *testing.T) {
	t.Parallel()

	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"schema", "--command", "config profile", "config", "profile"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestSchemaCommandRejectsUnknownPath(t *testing.T) {
	t.Parallel()

	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"schema", "nope"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestSchemaRootCoversCommandRegistry(t *testing.T) {
	t.Parallel()

	root := buildSchemaRoot()
	index := map[string]schemaCommand{}
	for _, sub := range root.Subcommands {
		index[sub.Name] = sub
	}

	for _, cmd := range rootCommands {
		node, ok := index[cmd.Name]
		if !ok {
			t.Fatalf("schema root missing command %q", cmd.Name)
		}

		subIndex := map[string]bool{}
		for _, sub := range node.Subcommands {
			subIndex[sub.Name] = true
		}
		for _, expected := range cmd.Subcommands {
			if !subIndex[expected] {
				t.Fatalf("schema command %q missing subcommand %q", cmd.Name, expected)
			}
		}
	}
}
