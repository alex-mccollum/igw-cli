package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvGatewayURL = "IGNITION_GATEWAY_URL"
	EnvToken      = "IGNITION_API_TOKEN"
)

type File struct {
	GatewayURL    string             `json:"gatewayURL,omitempty"`
	Token         string             `json:"token,omitempty"`
	ActiveProfile string             `json:"activeProfile,omitempty"`
	Profiles      map[string]Profile `json:"profiles,omitempty"`
}

type Profile struct {
	GatewayURL string `json:"gatewayURL,omitempty"`
	Token      string `json:"token,omitempty"`
}

type Effective struct {
	GatewayURL string `json:"gatewayURL,omitempty"`
	Token      string `json:"token,omitempty"`
	Profile    string `json:"profile,omitempty"`
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(base, "igw"), nil
}

func ResolveWithProfile(fileCfg File, getenv func(string) string, flagGatewayURL string, flagToken string, profile string) (Effective, error) {
	out := Effective{
		GatewayURL: strings.TrimSpace(fileCfg.GatewayURL),
		Token:      strings.TrimSpace(fileCfg.Token),
	}

	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = strings.TrimSpace(fileCfg.ActiveProfile)
	}
	if profile != "" {
		profileCfg, ok := fileCfg.Profiles[profile]
		if !ok {
			return Effective{}, fmt.Errorf("profile %q not found", profile)
		}

		out.GatewayURL = strings.TrimSpace(profileCfg.GatewayURL)
		out.Token = strings.TrimSpace(profileCfg.Token)
		out.Profile = profile
	}

	if v := strings.TrimSpace(getenv(EnvGatewayURL)); v != "" {
		out.GatewayURL = v
	}
	if v := strings.TrimSpace(getenv(EnvToken)); v != "" {
		out.Token = v
	}
	if v := strings.TrimSpace(flagGatewayURL); v != "" {
		out.GatewayURL = v
	}
	if v := strings.TrimSpace(flagToken); v != "" {
		out.Token = v
	}

	return out, nil
}
