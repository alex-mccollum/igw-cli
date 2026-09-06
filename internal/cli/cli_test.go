package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

const fixtureSpec = `{"openapi":"3.1.0","info":{"title":"Synthetic test API","version":"test"},"paths":{"/items/{name}":{"parameters":[{"in":"path","name":"name","required":true,"schema":{"type":"string"}}],"put":{"operationId":"updateItem","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["enabled"],"properties":{"enabled":{"type":"boolean"}}}}}},"responses":{"200":{"description":"OK"}}}},"/data/api/v1/gateway-info":{"get":{"operationId":"gatewayInfo","responses":{"200":{"description":"OK"}}}}}}`

func testApp(t *testing.T, srv *httptest.Server) (App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out, stderr := new(bytes.Buffer), new(bytes.Buffer)
	app := App{In: strings.NewReader(""), Out: out, Err: stderr, CacheDir: t.TempDir(), Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) { return config.File{}, nil }}
	if srv != nil {
		app.HTTP = srv.Client()
		app.ReadConfig = func() (config.File, error) {
			return config.File{GatewayURL: srv.URL + "/proxy", Token: "private-token"}, nil
		}
	}
	return app, out, stderr
}

func decodeResult(t *testing.T, out *bytes.Buffer) result.Result {
	t.Helper()
	var r result.Result
	d := json.NewDecoder(out)
	d.UseNumber()
	if err := d.Decode(&r); err != nil {
		t.Fatalf("invalid JSON result: %v", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		t.Fatal("extra output after result")
	}
	if r.Version != result.Version {
		t.Fatalf("wrong result contract: %+v", r)
	}
	return r
}

func TestSpecDiffSeparatesDocumentAndContractChanges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, after string
		equal       bool
	}{
		{"documentation", strings.ReplaceAll(fixtureSpec, `"description":"OK"`, `"description":"Updated help"`), true},
		{"constraint", strings.ReplaceAll(fixtureSpec, `"type":"boolean"`, `"type":"string"`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			before, after := filepath.Join(dir, "before.json"), filepath.Join(dir, "after.json")
			if err := os.WriteFile(before, []byte(fixtureSpec), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(after, []byte(tc.after), 0600); err != nil {
				t.Fatal(err)
			}
			app, out, _ := testApp(t, nil)
			if err := app.Run(context.Background(), []string{"spec", "diff", before, after, "--json"}); err != nil {
				t.Fatal(err)
			}
			data := decodeResult(t, out).Data.(map[string]any)
			if data["contractEqual"] != tc.equal || data["documentEqual"] != false || data["sharedOrPathDocumentChanged"] != true || len(data["changedOperationDocuments"].([]any)) == 0 {
				t.Fatalf("incorrect drift classification: %+v", data)
			}
			if (data["compatibility"] == "unchanged_under_policy") != tc.equal {
				t.Fatal("compatibility assessment disagrees with policy")
			}
		})
	}
}

func TestJSONIncludesParseErrorsAndOfflineCommandSchema(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--unknown", "--json"}, {"api", "request", "--json"}, {"schema", "--json"}, {"--help", "--json"}, {"completion", "bash", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			app, out, stderr := testApp(t, nil)
			app.ReadConfig = func() (config.File, error) {
				t.Fatal("offline command loaded configuration")
				return config.File{}, nil
			}
			err := app.Run(context.Background(), args)
			r := decodeResult(t, out)
			if (err == nil) != r.OK {
				t.Fatalf("error contract: %v %+v", err, r)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr)
			}
		})
	}
}

func TestPreviewAndValidationNeverSendMutation(t *testing.T) {
	t.Parallel()
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" && r.Method == "GET" {
			_, _ = w.Write([]byte(fixtureSpec))
			return
		}
		writes.Add(1)
		_, _ = w.Write([]byte(`{"applied":true}`))
	}))
	defer srv.Close()
	for _, tc := range []struct {
		name, body string
		flags      []string
		code       int
	}{
		{"preview", `{"enabled":true}`, []string{"--dry-run"}, 0},
		{"confirmation", `{"enabled":true}`, nil, 2},
		{"validation", `{"enabled":"private-value"}`, []string{"--yes"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, out, _ := testApp(t, srv)
			args := []string{"api", "request", "updateItem", "--path-param", "name=test", "--body", tc.body, "--json"}
			err := app.Run(context.Background(), append(args, tc.flags...))
			if igwerr.ExitCode(err) != tc.code {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(out.String(), "private-value") || strings.Contains(out.String(), "private-token") {
				t.Fatal("secret leaked")
			}
			r := decodeResult(t, out)
			if tc.name == "preview" && r.Outcome != "preview" {
				t.Fatalf("missing preview: %+v", r)
			}
		})
	}
	if writes.Load() != 0 {
		t.Fatal("preview or validation sent a proposed request")
	}
}

func TestExecutionPreservesJSONAndRefreshesWriteContract(t *testing.T) {
	t.Parallel()
	var specs, writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			specs.Add(1)
			_, _ = w.Write([]byte(fixtureSpec))
			return
		}
		if r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("managed credential missing")
		}
		if r.Method == "PUT" {
			writes.Add(1)
			if r.URL.EscapedPath() != "/proxy/items/a%2Fb%20c" {
				t.Errorf("incorrect path encoding: %s", r.URL.EscapedPath())
			}
		}
		_, _ = w.Write([]byte(`{"count":9007199254740993}`))
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	err := app.Run(context.Background(), []string{"api", "request", "updateItem", "--path-param", "name=a/b c", "--body", `{"enabled":true}`, "--yes", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	r := decodeResult(t, out)
	data, ok := r.Data.(map[string]any)
	if !ok || data["count"] != json.Number("9007199254740993") {
		t.Fatalf("API JSON was altered or double encoded: %v", r.Data)
	}
	if r.Outcome != "accepted" || r.Meta.Verification != "not_performed" || writes.Load() != 1 || specs.Load() < 2 {
		t.Fatalf("execution evidence wrong: %+v", r)
	}
}

func TestUnusableOperationSchemaFailsBeforeMutation(t *testing.T) {
	t.Parallel()
	// Reduced from the vendor's config/backupConfig duplicate schema IDs.
	schema := `{"type":"object","properties":{"config":{"$id":"urn:igw:test:duplicate","type":"object"},"backupConfig":{"$id":"urn:igw:test:duplicate","type":"object"}}}`
	spec := strings.Replace(fixtureSpec, `{"type":"object","required":["enabled"],"properties":{"enabled":{"type":"boolean"}}}`, schema, 1)
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/proxy/openapi.json" {
			_, _ = w.Write([]byte(spec))
			return
		}
		writes.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	for _, mode := range []string{"--dry-run", "--yes"} {
		app, out, stderr := testApp(t, srv)
		err := app.Run(context.Background(), []string{"api", "request", "updateItem", "--path-param", "name=test", "--body", `{"config":{"value":"private-input"}}`, mode, "--json"})
		if igwerr.ExitCode(err) != 2 || strings.Contains(out.String(), "private-input") || stderr.Len() != 0 {
			t.Fatalf("invalid schema failure contract: %v", err)
		}
		r := decodeResult(t, out)
		if r.OK || r.Error == nil || r.Error.Kind != "catalog_schema" {
			t.Fatalf("vendor defect was reported as a payload error: %+v", r)
		}
	}
	if writes.Load() != 0 {
		t.Fatal("an unvalidated operation was sent to the Gateway")
	}
}

func TestDoctorOnlyReads(t *testing.T) {
	t.Parallel()
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if strings.HasSuffix(r.URL.Path, "openapi.json") {
			_, _ = w.Write([]byte(fixtureSpec))
		} else {
			_, _ = w.Write([]byte(`{"name":"fixture"}`))
		}
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	if err := app.Run(context.Background(), []string{"gateway", "doctor", "--json"}); err != nil {
		t.Fatal(err)
	}
	if !decodeResult(t, out).OK {
		t.Fatal("doctor failed")
	}
	for _, method := range methods {
		if method != "GET" {
			t.Fatalf("doctor used %s", method)
		}
	}
}

func TestRawArtifactAndUncertainMutation(t *testing.T) {
	t.Parallel()
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			writes.Add(1)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte("\x00\xffartifact"))
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	path := filepath.Join(t.TempDir(), "archive")
	if err := app.Run(context.Background(), []string{"api", "raw", "--path", "/download", "--out", path, "--json"}); err != nil {
		t.Fatal(err)
	}
	r := decodeResult(t, out)
	if r.Artifact == nil || r.Artifact.Bytes != 10 {
		t.Fatalf("artifact metadata: %+v", r)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "\x00\xffartifact" {
		t.Fatalf("artifact bytes: %q %v", b, err)
	}
	out.Reset()
	err = app.Run(context.Background(), []string{"api", "raw", "--method", "POST", "--path", "/mutation", "--yes", "--json"})
	if igwerr.ExitCode(err) != 7 {
		t.Fatalf("uncertain mutation: %v", err)
	}
	r = decodeResult(t, out)
	if r.Outcome != "uncertain" || writes.Load() != 1 {
		t.Fatalf("uncertainty or retry contract failed: %+v, %d", r, writes.Load())
	}
}

func TestOfflineImportInspectAndPin(t *testing.T) {
	t.Parallel()
	app, out, _ := testApp(t, nil)
	path := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(path, []byte(fixtureSpec), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background(), []string{"spec", "inspect", path, "--json"}); err != nil {
		t.Fatal(err)
	}
	r := decodeResult(t, out)
	pin := r.Data.(map[string]any)["contractSha256"].(string)
	out.Reset()
	app.ReadConfig = func() (config.File, error) { return config.File{GatewayURL: "http://gateway.invalid"}, nil }
	if err := app.Run(context.Background(), []string{"spec", "import", path, "--json"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := app.Run(context.Background(), []string{"api", "describe", "updateItem", "--offline", "--spec-pin", pin, "--json"}); err != nil {
		t.Fatal(err)
	}
	r = decodeResult(t, out)
	if !r.Meta.Stale || r.Meta.Catalog.SourceKind != "import" {
		t.Fatalf("reference looked verified: %+v", r.Meta)
	}
}

func TestSpecSummaryProvidesIdentitiesWithoutOperationPayloads(t *testing.T) {
	t.Parallel()
	app, out, _ := testApp(t, nil)
	app.ReadConfig = func() (config.File, error) {
		t.Fatal("local inspection loaded configuration")
		return config.File{}, nil
	}
	path := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(path, []byte(fixtureSpec), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background(), []string{"spec", "inspect", path, "--summary", "--json"}); err != nil {
		t.Fatal(err)
	}
	r := decodeResult(t, out)
	data := r.Data.(map[string]any)
	for _, key := range []string{"rawSha256", "documentSha256", "contractSha256", "contractPolicy", "parserVersion", "operationCount"} {
		if data[key] == nil {
			t.Fatalf("summary omitted %s", key)
		}
	}
	if _, ok := data["operations"]; ok {
		t.Fatal("summary emitted all operation definitions")
	}
	if _, ok := data["adjustments"]; ok {
		t.Fatal("summary emitted detailed adjustments")
	}
}
