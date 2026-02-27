package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestAPICapabilityFileWrite(t *testing.T) {
	t.Parallel()

	spec := `{
  "openapi": "3.0.0",
  "paths": {
    "/data/api/v1/gateway-info": {
      "get": {
        "operationId": "gatewayInfo",
        "summary": "Gateway info"
      }
    },
    "/data/api/v1/scan/projects": {
      "post": {
        "operationId": "scanProjects",
        "summary": "Scan projects"
      }
    }
  }
}`

	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
	}

	if err := c.Execute([]string{"api", "capability", "file-write", "--spec-file", specPath, "--json"}); err != nil {
		t.Fatalf("api capability failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["classification"] != "api_supported" {
		t.Fatalf("unexpected classification %#v", payload["classification"])
	}
}
