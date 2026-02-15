package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandsAppearInUsageAndCompletion(t *testing.T) {
	t.Parallel()

	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	c.printRootUsage()
	usage := c.Err.(*bytes.Buffer).String()

	script := bashCompletionScript()

	for _, cmd := range rootCommands {
		if !strings.Contains(usage, cmd.Name) {
			t.Fatalf("usage missing command %q", cmd.Name)
		}
		if !strings.Contains(script, cmd.Name) {
			t.Fatalf("completion missing command %q", cmd.Name)
		}
	}

	if !strings.Contains(script, "help") {
		t.Fatalf("completion missing help command")
	}
}

func TestCompletionSubcommandsCoverRegistry(t *testing.T) {
	t.Parallel()

	script := bashCompletionScript()
	for cmd, subs := range completionSubcommands {
		if strings.TrimSpace(cmd) == "" {
			t.Fatalf("invalid empty completion subcommand key")
		}
		for _, sub := range subs {
			if !strings.Contains(script, sub) {
				t.Fatalf("completion script missing subcommand %q for %q", sub, cmd)
			}
		}
	}
}
