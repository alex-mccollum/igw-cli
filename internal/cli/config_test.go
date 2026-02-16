package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
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

func TestConfigSetAutoGateway(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var saved config.File

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		WriteConfig: func(cfg config.File) error {
			saved = cfg
			return nil
		},
		DetectWSLHostIP: func() (string, string, error) {
			return "172.25.80.1", "ip route default gateway", nil
		},
	}

	err := c.Execute([]string{"config", "set", "--auto-gateway"})
	if err != nil {
		t.Fatalf("config set auto-gateway failed: %v", err)
	}

	if saved.GatewayURL != "http://172.25.80.1:8088" {
		t.Fatalf("unexpected saved gateway url: %q", saved.GatewayURL)
	}

	if !strings.Contains(out.String(), "auto-detected gateway URL") {
		t.Fatalf("expected auto-detect output, got %q", out.String())
	}
}

func TestConfigSetAutoGatewayConflict(t *testing.T) {
	t.Parallel()

	c := &CLI{
		In:          strings.NewReader(""),
		Out:         new(bytes.Buffer),
		Err:         new(bytes.Buffer),
		Getenv:      func(string) string { return "" },
		ReadConfig:  func() (config.File, error) { return config.File{}, nil },
		WriteConfig: func(config.File) error { return nil },
	}

	err := c.Execute([]string{"config", "set", "--auto-gateway", "--gateway-url", "http://127.0.0.1:8088"})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestConfigSetAutoGatewayDetectFailure(t *testing.T) {
	t.Parallel()

	c := &CLI{
		In:          strings.NewReader(""),
		Out:         new(bytes.Buffer),
		Err:         new(bytes.Buffer),
		Getenv:      func(string) string { return "" },
		ReadConfig:  func() (config.File, error) { return config.File{}, nil },
		WriteConfig: func(config.File) error { return nil },
		DetectWSLHostIP: func() (string, string, error) {
			return "", "", errors.New("not in wsl")
		},
	}

	err := c.Execute([]string{"config", "set", "--auto-gateway"})
	if err == nil {
		t.Fatalf("expected detect error")
	}
	if !strings.Contains(err.Error(), "auto-gateway failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigSetJSONOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var saved config.File

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		WriteConfig: func(cfg config.File) error {
			saved = cfg
			return nil
		},
	}

	if err := c.Execute([]string{"config", "set", "--gateway-url", "http://127.0.0.1:8088", "--json"}); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	if saved.GatewayURL != "http://127.0.0.1:8088" {
		t.Fatalf("unexpected saved gateway URL: %q", saved.GatewayURL)
	}

	var payload struct {
		OK         bool   `json:"ok"`
		GatewayURL string `json:"gatewayURL"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v", err)
	}
	if !payload.OK || payload.GatewayURL != "http://127.0.0.1:8088" {
		t.Fatalf("unexpected json payload: %s", out.String())
	}
}

func TestConfigShowTextSortsProfiles(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{
				Profiles: map[string]config.Profile{
					"zeta":  {GatewayURL: "http://z", Token: "z-token"},
					"alpha": {GatewayURL: "http://a", Token: "a-token"},
				},
			}, nil
		},
	}

	if err := c.Execute([]string{"config", "show"}); err != nil {
		t.Fatalf("config show failed: %v", err)
	}

	got := out.String()
	alpha := strings.Index(got, "profile\talpha\t")
	zeta := strings.Index(got, "profile\tzeta\t")
	if alpha == -1 || zeta == -1 || alpha > zeta {
		t.Fatalf("profiles not sorted in text output: %q", got)
	}
}
