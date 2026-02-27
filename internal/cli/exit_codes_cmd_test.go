package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestExitCodesCommandText(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"exit-codes"}); err != nil {
		t.Fatalf("exit-codes failed: %v", err)
	}

	got := out.String()
	for _, line := range []string{
		"auth\t6",
		"network\t7",
		"ok\t0",
		"usage\t2",
	} {
		if !strings.Contains(got, line) {
			t.Fatalf("expected line %q in output %q", line, got)
		}
	}
}

func TestExitCodesCommandJSON(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"exit-codes", "--json"}); err != nil {
		t.Fatalf("exit-codes --json failed: %v", err)
	}

	var payload struct {
		OK        bool           `json:"ok"`
		ExitCodes map[string]int `json:"exitCodes"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true payload")
	}
	if payload.ExitCodes["ok"] != 0 || payload.ExitCodes["usage"] != 2 || payload.ExitCodes["auth"] != 6 || payload.ExitCodes["network"] != 7 {
		t.Fatalf("unexpected exit codes payload: %#v", payload.ExitCodes)
	}
}

func TestExitCodesCommandRejectsArgs(t *testing.T) {
	t.Parallel()

	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"exit-codes", "extra"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
