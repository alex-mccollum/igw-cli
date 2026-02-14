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
