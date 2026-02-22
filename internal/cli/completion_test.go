package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestCompletionBash(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"completion", "bash"}); err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	script := out.String()
	if !strings.Contains(script, "_igw_completion") {
		t.Fatalf("missing completion function in script")
	}
	if !strings.Contains(script, "igw config profile list") {
		t.Fatalf("missing profile-aware completion in script")
	}
	if !strings.Contains(script, "version") {
		t.Fatalf("missing version completion entry")
	}
	if !strings.Contains(script, "list download loggers logger level-reset") || !strings.Contains(script, "generate status download") {
		t.Fatalf("missing new command completion entries")
	}
	if !strings.Contains(script, "sync") || !strings.Contains(script, "refresh") || !strings.Contains(script, "diagnostics-bundle") || !strings.Contains(script, "restart-tasks") {
		t.Fatalf("missing api/wait completion entries")
	}
	if !strings.Contains(script, "--select") || !strings.Contains(script, "--raw") {
		t.Fatalf("missing --select/--raw completion flags")
	}
	if !strings.Contains(script, "--compact") {
		t.Fatalf("missing --compact completion flag")
	}
	if !strings.Contains(script, "--prefix-depth") {
		t.Fatalf("missing --prefix-depth completion flag")
	}
	if strings.Contains(script, "--field") || strings.Contains(script, "--fields") {
		t.Fatalf("found deprecated --field/--fields completion flags")
	}
}

func TestCompletionUnsupportedShell(t *testing.T) {
	t.Parallel()

	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"completion", "zsh"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
