package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const apiSpecFixture = `{
  "openapi": "3.0.0",
  "paths": {
    "/data/api/v1/scan/projects": {
      "post": {
        "operationId": "scanProjects",
        "summary": "Scan projects",
        "description": "Trigger a scan",
        "tags": ["gateway", "scan"]
      }
    },
    "/data/api/v1/gateway-info": {
      "get": {
        "operationId": "gatewayInfo",
        "summary": "Gateway info",
        "description": "Get gateway info",
        "tags": ["gateway"]
      }
    }
  }
}`

func TestAPIList(t *testing.T) {
	t.Parallel()

	specPath := writeAPISpec(t, apiSpecFixture)
	var out bytes.Buffer
	var errOut bytes.Buffer

	c := &CLI{
		Out: &out,
		Err: &errOut,
	}

	err := c.Execute([]string{"api", "list", "--spec-file", specPath})
	if err != nil {
		t.Fatalf("api list failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "GET\t/data/api/v1/gateway-info\tgatewayInfo") {
		t.Fatalf("missing gateway info operation: %q", got)
	}
	if !strings.Contains(got, "POST\t/data/api/v1/scan/projects\tscanProjects") {
		t.Fatalf("missing scan operation: %q", got)
	}
}

func TestAPIShowMissingPath(t *testing.T) {
	t.Parallel()

	specPath := writeAPISpec(t, apiSpecFixture)
	c := &CLI{
		Out: new(bytes.Buffer),
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"api", "show", "--spec-file", specPath, "--path", "/does/not/exist"})
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}

func TestAPISearchJSON(t *testing.T) {
	t.Parallel()

	specPath := writeAPISpec(t, apiSpecFixture)
	var out bytes.Buffer

	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"api", "search", "--spec-file", specPath, "--query", "scan", "--json"})
	if err != nil {
		t.Fatalf("api search failed: %v", err)
	}

	var payload struct {
		Count      int `json:"count"`
		Operations []struct {
			OperationID string `json:"operationId"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v", err)
	}
	if payload.Count != 1 || len(payload.Operations) != 1 {
		t.Fatalf("unexpected count payload: %+v", payload)
	}
	if payload.Operations[0].OperationID != "scanProjects" {
		t.Fatalf("unexpected operation id: %+v", payload.Operations[0])
	}
}

func TestAPIShowAcceptsPositionalPath(t *testing.T) {
	t.Parallel()

	specPath := writeAPISpec(t, apiSpecFixture)
	var out bytes.Buffer

	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"api", "show", "--spec-file", specPath, "/data/api/v1/gateway-info"})
	if err != nil {
		t.Fatalf("api show failed: %v", err)
	}

	if !strings.Contains(out.String(), "operation_id\tgatewayInfo") {
		t.Fatalf("expected gatewayInfo output, got %q", out.String())
	}
}

func TestAPIShowJSONErrorEnvelope(t *testing.T) {
	t.Parallel()

	specPath := writeAPISpec(t, apiSpecFixture)
	var out bytes.Buffer

	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	err := c.Execute([]string{"api", "show", "--spec-file", specPath, "--json"})
	if err == nil {
		t.Fatalf("expected usage error")
	}

	var payload struct {
		OK    bool   `json:"ok"`
		Code  int    `json:"code"`
		Error string `json:"error"`
	}
	if unmarshalErr := json.Unmarshal(out.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("parse json output: %v", unmarshalErr)
	}
	if payload.OK || payload.Code != 2 || !strings.Contains(payload.Error, "required: --path") {
		t.Fatalf("unexpected json error payload: %s", out.String())
	}
}

func TestAPIFallbackToConfigDirSpec(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	specDir := filepath.Join(configHome, "igw")
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		t.Fatalf("create config spec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "openapi.json"), []byte(apiSpecFixture), 0o600); err != nil {
		t.Fatalf("write config spec: %v", err)
	}

	emptyDir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	var out bytes.Buffer
	c := &CLI{
		Out: &out,
		Err: new(bytes.Buffer),
	}

	if err := c.Execute([]string{"api", "list"}); err != nil {
		t.Fatalf("api list failed: %v", err)
	}

	if !strings.Contains(out.String(), "gatewayInfo") {
		t.Fatalf("expected operation from config-dir spec, got %q", out.String())
	}
}

func writeAPISpec(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	return path
}
