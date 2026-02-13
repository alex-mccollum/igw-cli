package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestConfigShowMasksToken(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errOut bytes.Buffer

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    &errOut,
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{
				GatewayURL: "http://127.0.0.1:8088",
				Token:      "abcd1234xyz",
			}, nil
		},
	}

	if err := c.Execute([]string{"config", "show"}); err != nil {
		t.Fatalf("execute config show: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "abcd1234xyz") {
		t.Fatalf("raw token leaked in output: %q", got)
	}
	if !strings.Contains(got, "abcd...yz") {
		t.Fatalf("masked token missing in output: %q", got)
	}
}
