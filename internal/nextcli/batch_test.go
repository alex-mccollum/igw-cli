package nextcli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/execute"
)

func TestBatchCLIExactInputsAndPreview(t *testing.T) {
	const body = `{ "enabled": true, "count": 9007199254740993 }`
	const name = "name/ + & 日本"
	const input = `[{"id":"before","operation":"gatewayInfo"},{"id":"change","operation":"updateItem","pathParams":{"name":"name/ + & 日本"},"query":{"tag":[" a+b & = 日本 ",""]},"headers":{"X-Test":["one","two"]},"body":` + body + `},{"id":"after","operation":"gatewayInfo"}]`
	var specs, operations, writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("missing managed credential")
		}
		if r.URL.Path == "/proxy/openapi.json" {
			specs.Add(1)
			_, _ = io.WriteString(w, fixtureSpec)
			return
		}
		operations.Add(1)
		if r.Method == "PUT" {
			writes.Add(1)
			data, _ := io.ReadAll(r.Body)
			if string(data) != body || r.URL.EscapedPath() != "/proxy/items/"+url.PathEscape(name) || !reflect.DeepEqual(r.URL.Query()["tag"], []string{" a+b & = 日本 ", ""}) || !reflect.DeepEqual(r.Header.Values("X-Test"), []string{"one", "two"}) {
				t.Error("batch changed wire inputs")
			}
		}
		_, _ = io.WriteString(w, `{"count":9007199254740993}`)
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	for _, source := range []string{"inline", "file", "stdin"} {
		selector := input
		switch source {
		case "file":
			path := filepath.Join(t.TempDir(), "batch.json")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			selector = "@" + path
		case "stdin":
			app.In = strings.NewReader(input)
			selector = "-"
		}
		mode := "--yes"
		if source == "inline" {
			mode = "--dry-run"
		}
		if err := app.Run(context.Background(), []string{"api", "batch", "--input", selector, mode, "--json"}); err != nil {
			t.Fatal(err)
		}
		encoded := out.String()
		if strings.Contains(encoded, "private-token") || stderr.Len() != 0 {
			t.Fatal("batch leaked credentials or emitted extra stderr")
		}
		got := decodeResult(t, out)
		raw, _ := json.Marshal(got.Data)
		var report execute.BatchReport
		if json.Unmarshal(raw, &report) != nil || report.Succeeded != 3 || report.Failed != 0 || report.NotRun != 0 || len(report.Items) != 3 {
			t.Fatalf("incomplete batch report: %s", raw)
		}
		if source == "inline" {
			if got.Outcome != "preview" || operations.Load() != 0 || strings.Contains(encoded, "9007199254740993") {
				t.Fatal("preview dispatched or exposed body values")
			}
			raw, _ = json.Marshal(report.Items[1].Result.Data)
			var p execute.Preview
			if json.Unmarshal(raw, &p) != nil || p.BodySHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) || !p.BodyPresent {
				t.Fatal("batch preview lost exact body identity")
			}
		} else if got.Outcome != "accepted" || !strings.Contains(encoded, "9007199254740993") {
			t.Fatal("batch lost accepted outcome or response precision")
		}
	}
	if err := app.Run(context.Background(), []string{"api", "batch", "--input", input, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), fmt.Sprintf("%x", sha256.Sum256([]byte(body)))) || !strings.Contains(out.String(), "change\tpreview") || strings.Contains(out.String(), "9007199254740993") {
		t.Fatal("human batch preview omitted request identity or exposed body values")
	}
	if specs.Load() != 3 || operations.Load() != 6 || writes.Load() != 2 {
		t.Fatalf("unexpected requests: %d/%d/%d", specs.Load(), operations.Load(), writes.Load())
	}
}

func TestBatchCLIPartialHumanAndJSONResults(t *testing.T) {
	var reads, writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, fixtureSpec)
			return
		}
		if r.Method == "GET" {
			reads.Add(1)
		} else {
			writes.Add(1)
		}
		_, _ = io.WriteString(w, `{"before":true}`)
	}))
	defer srv.Close()
	const input = `[{"id":"before","operation":"gatewayInfo"},{"id":"bad","operation":"updateItem","pathParams":{"name":"test"},"body":{"enabled":"private-bad-value"}},{"id":"after","operation":"gatewayInfo"}]`
	for _, machine := range []bool{false, true} {
		app, out, stderr := testApp(t, srv)
		args := []string{"api", "batch", "--input", input, "--yes"}
		if machine {
			args = append(args, "--json")
		}
		if err := app.Run(context.Background(), args); err == nil {
			t.Fatal("batch hid partial failure")
		}
		if strings.Contains(out.String()+stderr.String(), "private-bad-value") {
			t.Fatal("invalid body leaked")
		}
		if machine {
			got := decodeResult(t, out)
			if got.OK || got.Outcome != "partial" || got.Error.Code != 2 || stderr.Len() != 0 {
				t.Fatal("partial result contract changed")
			}
			data := got.Data.(map[string]any)
			items := data["items"].([]any)
			if len(items) != 3 || items[0].(map[string]any)["result"].(map[string]any)["data"].(map[string]any)["before"] != true || items[2].(map[string]any)["result"].(map[string]any)["outcome"] != "not_run" {
				t.Fatal("partial results lost")
			}
		} else if !strings.Contains(out.String(), "before\tcompleted") || !strings.Contains(out.String(), "bad\tfailed\tvalidation") || !strings.Contains(out.String(), "after\tnot_run") || !strings.Contains(stderr.String(), "failed items") {
			t.Fatal("human output hid per-item results")
		}
	}
	if reads.Load() != 2 || writes.Load() != 0 {
		t.Fatal("invalid or later item dispatched")
	}
}

func TestBatchDecodeRejectsMalformedManifestBeforeRuntime(t *testing.T) {
	for _, input := range []string{
		`null`, `[]`, `{}`, `[null]`, `[{"id":"a","operation":"x"}] true`,
		`[{"id":"a","ID":"b","operation":"x"}]`,
		`[{"id":"a","id":"b","operation":"x"}]`,
		`[{"id":"a","operation":"x"},{"id":"a","operation":"y"}]`,
		`[{"id":"a","operation":"x","yes":true}]`,
		`[{"id":"a","operation":"x","body":null,"bodyText":""}]`,
		`[{"id":"a","operation":"x","body":{"secret":1,"secret":2}}]`,
		`[{"id":"a","operation":"x","bodyText":null}]`,
		`[{"id":"a","operation":"x","bodyText":"\ud800"}]`,
		`[{"id":"a","operation":"x","query":null}]`,
		`[{"id":"a","operation":"x","query":{"key":null}}]`,
		`[{"id":"a","operation":"x","query":{"key":[null]}}]`,
		`[{"id":"a","operation":"x","headers":{"key":[5]}}]`,
		`[{"id":"a","operation":"x","pathParams":{"key":null}}]`,
		`[{"id":"a","operation":"x","upload":"private-file"}]`,
		`[{"id":"a","operation":"x","out":"private-file"}]`,
		`[{"id":"a","operation":"x","contentType":""}]`,
		`[{"id":"a","operation":"x"},{"id":"b","operation":"x","unknown":true}]`,
		strings.Repeat(" ", execute.MaxBatchInputBytes+1),
	} {
		app, out, _ := testApp(t, nil)
		app.ReadConfig = func() (config.File, error) { t.Fatal("malformed batch reached runtime"); return config.File{}, nil }
		if err := app.Run(context.Background(), []string{"api", "batch", "--input", input, "--yes", "--json"}); err == nil {
			t.Fatal("malformed manifest accepted")
		}
		got := decodeResult(t, out)
		if got.Error.Code != 2 {
			t.Fatal("malformed manifest has unstable exit code")
		}
	}
}

func TestBatchDecodePreservesNullEmptyAndLiteralText(t *testing.T) {
	items, err := decodeBatch([]byte(`[{"id":"none","operation":"x"},{"id":"null","operation":"x","body":null},{"id":"empty","operation":"x","bodyText":""},{"id":"literal","operation":"x","bodyText":"@file - & 日本","query":{"present":[""],"absent":[]}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Request.Body != nil || string(items[1].Request.Body) != "null" || items[2].Request.Body == nil || len(items[2].Request.Body) != 0 || string(items[3].Request.Body) != "@file - & 日本" || !reflect.DeepEqual(items[3].Request.Query["present"], []string{""}) {
		t.Fatal("batch input intent changed")
	}
}

func TestBatchCommandSchemaIsOffline(t *testing.T) {
	app, out, _ := testApp(t, nil)
	app.ReadConfig = func() (config.File, error) { t.Fatal("schema touched runtime"); return config.File{}, nil }
	if err := app.Run(context.Background(), []string{"api", "batch", "--help", "--json"}); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	data := got.Data.(map[string]any)
	found := false
	for _, entry := range data["flags"].([]any) {
		flag := entry.(map[string]any)
		if flag["name"] == "input" {
			schema, ok := flag["inputSchema"].(map[string]any)
			found = ok && schema["type"] == "array" && flag["required"] == true
		}
	}
	if !found {
		t.Fatal("batch input schema missing")
	}
}

func TestBatchExpandedInputsFailBeforeRuntime(t *testing.T) {
	for _, location := range []string{"query", "headers"} {
		t.Run(location, func(t *testing.T) {
			key := strings.Repeat("x", 4096)
			values := make([]string, execute.MaxBatchInputBytes/len(key)+1)
			input, err := json.Marshal([]map[string]any{{"id": "read", "operation": "GET /unused", location: map[string][]string{key: values}}})
			if err != nil || len(input) >= execute.MaxBatchInputBytes {
				t.Fatal("fixture must be a small valid JSON manifest")
			}
			app, out, _ := testApp(t, nil)
			app.ReadConfig = func() (config.File, error) { t.Fatal("expanded inputs reached runtime"); return config.File{}, nil }
			if err := app.Run(context.Background(), []string{"api", "batch", "--input", string(input), "--json"}); err == nil {
				t.Fatal("expanded input was accepted")
			}
			if got := decodeResult(t, out); got.Error.Code != 2 {
				t.Fatal("expanded input refusal changed exit code")
			}
		})
	}
}
