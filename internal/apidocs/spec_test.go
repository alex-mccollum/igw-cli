package apidocs

import (
	"os"
	"path/filepath"
	"testing"
)

const testSpec = `{
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
      },
      "parameters": []
    }
  }
}`

func TestLoadOperations(t *testing.T) {
	t.Parallel()

	specPath := writeSpec(t, testSpec)
	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("load operations: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}

	if ops[0].Path != "/data/api/v1/gateway-info" || ops[0].Method != "GET" {
		t.Fatalf("unexpected first operation: %+v", ops[0])
	}
	if ops[1].Path != "/data/api/v1/scan/projects" || ops[1].Method != "POST" {
		t.Fatalf("unexpected second operation: %+v", ops[1])
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()

	specPath := writeSpec(t, testSpec)
	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("load operations: %v", err)
	}

	got := Search(ops, "scan")
	if len(got) != 1 {
		t.Fatalf("expected one search result, got %d", len(got))
	}
	if got[0].OperationID != "scanProjects" {
		t.Fatalf("unexpected search result: %+v", got[0])
	}
}

func TestFilterByOperationID(t *testing.T) {
	t.Parallel()

	specPath := writeSpec(t, testSpec)
	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("load operations: %v", err)
	}

	got := FilterByOperationID(ops, "gatewayInfo")
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Path != "/data/api/v1/gateway-info" || got[0].Method != "GET" {
		t.Fatalf("unexpected operation: %+v", got[0])
	}

	insensitive := FilterByOperationID(ops, "gatewayinfo")
	if len(insensitive) != 1 {
		t.Fatalf("expected case-insensitive fallback to match one result, got %d", len(insensitive))
	}
}

func TestUniqueTags(t *testing.T) {
	t.Parallel()

	specPath := writeSpec(t, testSpec)
	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("load operations: %v", err)
	}

	got := UniqueTags(ops)
	if len(got) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(got))
	}
	if got[0] != "gateway" || got[1] != "scan" {
		t.Fatalf("unexpected tags: %+v", got)
	}
}

func TestBuildStats(t *testing.T) {
	t.Parallel()

	specPath := writeSpec(t, testSpec)
	ops, err := LoadOperations(specPath)
	if err != nil {
		t.Fatalf("load operations: %v", err)
	}

	stats := BuildStats(ops)
	if stats.Total != 2 {
		t.Fatalf("unexpected total: %d", stats.Total)
	}

	if len(stats.Methods) != 2 {
		t.Fatalf("unexpected methods length: %d", len(stats.Methods))
	}
	if stats.Methods[0].Name != "GET" || stats.Methods[0].Count != 1 {
		t.Fatalf("unexpected GET method stat: %+v", stats.Methods[0])
	}
	if stats.Methods[1].Name != "POST" || stats.Methods[1].Count != 1 {
		t.Fatalf("unexpected POST method stat: %+v", stats.Methods[1])
	}

	if len(stats.PathPrefixes) != 2 {
		t.Fatalf("unexpected path prefix length: %d", len(stats.PathPrefixes))
	}
	if stats.PathPrefixes[0].Name != "/data/api/v1/gateway-info" || stats.PathPrefixes[0].Count != 1 {
		t.Fatalf("unexpected first path prefix stat: %+v", stats.PathPrefixes[0])
	}
	if stats.PathPrefixes[1].Name != "/data/api/v1/scan" || stats.PathPrefixes[1].Count != 1 {
		t.Fatalf("unexpected second path prefix stat: %+v", stats.PathPrefixes[1])
	}
}

func writeSpec(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}
