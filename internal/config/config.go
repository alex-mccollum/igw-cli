package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	EnvGatewayURL = "IGNITION_GATEWAY_URL"
	EnvToken      = "IGNITION_API_TOKEN"
)

type File struct {
	GatewayURL string `json:"gatewayURL,omitempty"`
	Token      string `json:"token,omitempty"`
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(base, "igw"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

func Read() (File, error) {
	path, err := Path()
	if err != nil {
		return File{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec // config path is fixed
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json.Unmarshal(b, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func Write(cfg File) error {
	dir, err := Dir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	path, err := Path()
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	b = append(b, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write config temp: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}

	return nil
}

func Resolve(fileCfg File, getenv func(string) string, flagGatewayURL string, flagToken string) File {
	out := fileCfg

	if v := getenv(EnvGatewayURL); v != "" {
		out.GatewayURL = v
	}
	if v := getenv(EnvToken); v != "" {
		out.Token = v
	}
	if flagGatewayURL != "" {
		out.GatewayURL = flagGatewayURL
	}
	if flagToken != "" {
		out.Token = flagToken
	}

	return out
}

func MaskToken(token string) string {
	if token == "" {
		return ""
	}

	if len(token) <= 4 {
		return "****"
	}

	if len(token) <= 8 {
		return token[:2] + "..." + token[len(token)-1:]
	}

	return token[:4] + "..." + token[len(token)-2:]
}
