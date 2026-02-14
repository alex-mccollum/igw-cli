package config

import "testing"

func TestResolvePrecedence(t *testing.T) {
	t.Parallel()

	fileCfg := File{
		GatewayURL: "http://from-file:8088",
		Token:      "file-token",
	}
	getenv := func(key string) string {
		switch key {
		case EnvGatewayURL:
			return "http://from-env:8088"
		case EnvToken:
			return "env-token"
		default:
			return ""
		}
	}

	resolved := Resolve(fileCfg, getenv, "http://from-flag:8088", "flag-token")
	if resolved.GatewayURL != "http://from-flag:8088" {
		t.Fatalf("gateway precedence failed: got %q", resolved.GatewayURL)
	}
	if resolved.Token != "flag-token" {
		t.Fatalf("token precedence failed: got %q", resolved.Token)
	}
}

func TestResolveWithProfile(t *testing.T) {
	t.Parallel()

	fileCfg := File{
		GatewayURL:    "http://default:8088",
		Token:         "default-token",
		ActiveProfile: "prod",
		Profiles: map[string]Profile{
			"prod": {
				GatewayURL: "http://prod:8088",
				Token:      "prod-token",
			},
		},
	}

	getenv := func(string) string { return "" }

	resolved, err := ResolveWithProfile(fileCfg, getenv, "", "", "")
	if err != nil {
		t.Fatalf("resolve with active profile: %v", err)
	}
	if resolved.GatewayURL != "http://prod:8088" {
		t.Fatalf("unexpected gateway url %q", resolved.GatewayURL)
	}
	if resolved.Token != "prod-token" {
		t.Fatalf("unexpected token %q", resolved.Token)
	}
	if resolved.Profile != "prod" {
		t.Fatalf("unexpected profile %q", resolved.Profile)
	}
}

func TestResolveWithProfileUnknown(t *testing.T) {
	t.Parallel()

	fileCfg := File{
		Profiles: map[string]Profile{
			"dev": {GatewayURL: "http://dev:8088", Token: "dev-token"},
		},
	}

	_, err := ResolveWithProfile(fileCfg, func(string) string { return "" }, "", "", "missing")
	if err == nil {
		t.Fatalf("expected missing profile error")
	}
}
