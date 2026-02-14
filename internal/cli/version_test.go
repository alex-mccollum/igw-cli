package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"version"}); err != nil {
		t.Fatalf("version failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "igw version ") {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestVersionCommandRejectsArgs(t *testing.T) {
	t.Parallel()

	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"version", "extra"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
