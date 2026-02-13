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
