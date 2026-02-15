package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestConfigProfileAddUseList(t *testing.T) {
	t.Parallel()

	var cfg config.File
	var out bytes.Buffer
	var errOut bytes.Buffer

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    &errOut,
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return cfg, nil
		},
		WriteConfig: func(next config.File) error {
			cfg = next
			return nil
		},
	}

	if err := c.Execute([]string{
		"config", "profile", "add", "dev",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "dev-token",
		"--use",
	}); err != nil {
		t.Fatalf("profile add failed: %v", err)
	}

	if cfg.ActiveProfile != "dev" {
		t.Fatalf("expected active profile dev, got %q", cfg.ActiveProfile)
	}
	if cfg.Profiles["dev"].GatewayURL != "http://127.0.0.1:8088" {
		t.Fatalf("unexpected profile gateway: %+v", cfg.Profiles["dev"])
	}

	out.Reset()
	if err := c.Execute([]string{"config", "profile", "list"}); err != nil {
		t.Fatalf("profile list failed: %v", err)
	}
	if !strings.Contains(out.String(), "*\tdev\thttp://127.0.0.1:8088\t") {
		t.Fatalf("expected active profile row, got %q", out.String())
	}
}

func TestConfigProfileAddFirstProfileBecomesActive(t *testing.T) {
	t.Parallel()

	var cfg config.File
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    new(bytes.Buffer),
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return cfg, nil
		},
		WriteConfig: func(next config.File) error {
			cfg = next
			return nil
		},
	}

	if err := c.Execute([]string{
		"config", "profile", "add", "dev",
		"--gateway-url", "http://127.0.0.1:8088",
		"--api-key", "dev-token",
	}); err != nil {
		t.Fatalf("profile add failed: %v", err)
	}

	if cfg.ActiveProfile != "dev" {
		t.Fatalf("expected first profile to become active, got %q", cfg.ActiveProfile)
	}
}
