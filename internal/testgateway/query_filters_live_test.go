package testgateway_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/nextcli"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

// The query suite preserves independent evidence and does not relabel an older
// workflow reference. It exercises both successful filtering and refusal at
// the actual CLI/HTTP boundary, using only an explicitly disposable Gateway.
func TestLiveQueryFilters(t *testing.T) {
	image := os.Getenv("IGW_ACCEPTANCE_TEST_IMAGE")
	if image == "" {
		t.Skip("requires a pinned image and guarded live invocation")
	}
	dir := os.Getenv("IGW_QUERY_EVIDENCE_DIR")
	if dir == "" {
		t.Fatal("requires a new IGW_QUERY_EVIDENCE_DIR")
	}
	cfg, err := testgateway.ProfileConfig(image, os.Getenv("IGW_CAPTURE_TEST_DOCKER"), os.Getenv("IGW_TEST_MODULE_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	receipt := struct {
		Version          int                          `json:"version"`
		Kind             string                       `json:"kind"`
		Image            string                       `json:"image"`
		ImageID          string                       `json:"imageId"`
		Platform         string                       `json:"platform"`
		GatewayVersion   string                       `json:"gatewayVersion"`
		ModuleInventory  *testgateway.ModuleInventory `json:"moduleInventory,omitempty"`
		ModuleWhitelist  []string                     `json:"moduleWhitelist,omitempty"`
		TestBinarySHA256 string                       `json:"testBinarySha256"`
		StartedAt        time.Time                    `json:"startedAt"`
		FinishedAt       time.Time                    `json:"finishedAt"`
		Catalog          *catalog.Metadata            `json:"catalog,omitempty"`
		OpenAPI          *artifact.Info               `json:"openapi,omitempty"`
		Checks           []transferCheck              `json:"checks"`
		Cleanup          bool                         `json:"cleanup"`
		Passed           bool                         `json:"passed"`
	}{Version: 1, Kind: "query-filter-workflows", Image: image, StartedAt: time.Now().UTC(), Checks: []transferCheck{}}
	var session *testgateway.Session
	defer func() {
		if session != nil {
			if err := session.Close(); err != nil {
				t.Errorf("owned query Gateway cleanup: %v", err)
			} else {
				receipt.Cleanup = true
			}
			receipt.ImageID, receipt.Platform, receipt.GatewayVersion = session.ImageID, session.Platform, session.GatewayVersion
			receipt.ModuleInventory, receipt.ModuleWhitelist = session.ModuleInventory, session.Modules
		}
		receipt.FinishedAt = time.Now().UTC()
		receipt.Passed = !t.Failed() && receipt.Cleanup
		b, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		w, err := artifact.New(filepath.Join(dir, "query-filters.json"), false)
		if err != nil {
			t.Error(err)
			return
		}
		defer w.Abort()
		if _, err := w.Write(append(b, '\n')); err != nil {
			t.Error(err)
			return
		}
		if _, err := w.Commit(); err != nil {
			t.Error(err)
		}
	}()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, hashErr := io.Copy(hash, f)
	closeErr := f.Close()
	if hashErr != nil || closeErr != nil {
		t.Fatal("cannot hash query acceptance executable")
	}
	receipt.TestBinarySHA256 = hex.EncodeToString(hash.Sum(nil))
	session, err = testgateway.Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.WaitOpenAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.ValidateModuleProfile(); err != nil {
		t.Fatal(err)
	}
	token, err := session.ProvisionAPIToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, secret, _ := strings.Cut(token, ":")
	cache := t.TempDir()
	client := session.HTTPClient()
	var requests atomic.Int64
	client.Transport = transferTransport{base: client.Transport, operations: &requests}
	run := func(name string, args ...string) transferResult {
		t.Helper()
		requests.Store(0)
		var out, stderr bytes.Buffer
		app := nextcli.App{In: strings.NewReader(""), Out: &out, Err: &stderr, CacheDir: cache, HTTP: client, Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) { return config.File{GatewayURL: session.URL, Token: token}, nil }}
		err := app.Run(ctx, append(args, "--json", "--timeout", "90s"))
		if strings.Contains(out.String(), token) || strings.Contains(stderr.String(), token) || (secret != "" && (strings.Contains(out.String(), secret) || strings.Contains(stderr.String(), secret))) {
			t.Fatal("query CLI leaked its credential")
		}
		var got transferResult
		if json.Unmarshal(out.Bytes(), &got) != nil || got.OK != (err == nil) {
			t.Fatalf("%s invalid CLI envelope", name)
		}
		check := transferCheck{Name: name, Outcome: got.Outcome, HTTPStatus: got.Meta.HTTPStatus, OperationRequests: requests.Load()}
		if got.Error != nil {
			check.ErrorKind, check.ExitCode = got.Error.Kind, got.Error.Code
		}
		receipt.Checks = append(receipt.Checks, check)
		t.Logf("%s: %s HTTP=%d requests=%d", name, check.Outcome, check.HTTPStatus, check.OperationRequests)
		if got.Meta.Catalog != nil && receipt.Catalog != nil && (got.Meta.Catalog.ContractSHA256 != receipt.Catalog.ContractSHA256 || got.Meta.Catalog.ParserVersion != catalog.ParserVersion) {
			t.Fatal("query invocation changed the captured contract or parser")
		}
		return got
	}
	ok := func(name string, args ...string) transferResult {
		t.Helper()
		got := run(name, args...)
		if !got.OK {
			t.Fatalf("%s: %v", name, got.Error)
		}
		return got
	}
	synced := ok("catalog", "spec", "sync")
	if synced.Meta.Catalog == nil || synced.Meta.Target == nil {
		t.Fatal("query catalog provenance missing")
	}
	receipt.Catalog = synced.Meta.Catalog
	snapshot, err := (catalog.Store{Dir: cache}).Load(*synced.Meta.Target)
	if err != nil {
		t.Fatal(err)
	}
	raw := snapshot.Catalog.Raw()
	if snapshot.Metadata.RawSHA256 != receipt.Catalog.RawSHA256 {
		snapshot.Close()
		t.Fatal("query catalog export changed snapshot")
	}
	snapshot.Close()
	w, err := artifact.New(filepath.Join(dir, "openapi.json.gz"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	gz := gzip.NewWriter(w)
	if _, err := gz.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := w.Commit()
	if err != nil {
		t.Fatal(err)
	}
	receipt.OpenAPI = &info

	type item struct {
		Name, Description, LoggerName string
		Enabled                       *bool
	}
	type page struct {
		Items    []item
		Metadata struct{ Matching, Limit, Offset float64 }
	}
	readPage := func(name string, args ...string) page {
		t.Helper()
		got := ok(name, args...)
		if got.Meta.HTTPStatus != 200 || requests.Load() != 1 {
			t.Fatalf("%s did not make exactly one successful list request", name)
		}
		var value page
		if json.Unmarshal(got.Data, &value) != nil || value.Items == nil {
			t.Fatalf("%s malformed list", name)
		}
		return value
	}
	one := func(p page, name string) {
		t.Helper()
		if len(p.Items) != 1 || p.Items[0].Name != name || p.Metadata.Matching != 1 {
			t.Fatal("filter did not select exactly the controlled item")
		}
	}
	const alpha, beta = "igw-filter-alpha", "igw-filter-beta"
	const special = "filter + & % # / = 日本"
	for _, name := range []string{alpha, beta} {
		description := special
		if name == beta {
			description = "another value"
		}
		body, _ := json.Marshal(map[string]any{"description": description, "enabled": true, "config": map[string]any{"profile": map[string]any{"type": "basic schedule"}, "settings": map[string]any{"allDays": true, "allDayTime": "01:00-02:00"}}})
		created := ok("create-resource-"+name, "resource", "create", "ignition/schedule", name, "--body", string(body), "--yes")
		if created.Outcome != "completed" || created.Meta.Verification != "verified" {
			t.Fatal("query resource setup was not verified")
		}
	}
	one(readPage("resource-name-eq", "resource", "list", "ignition/schedule", "--filter", "name[eq]="+alpha), alpha)
	one(readPage("resource-exact-description", "resource", "list", "ignition/schedule", "--filter", "description[eq]="+special), alpha)
	both := readPage("resource-multiple-filters", "resource", "list", "ignition/schedule", "--filter", "name[sw]=igw-filter-", "--filter", "enabled[eq]=true")
	if len(both.Items) != 2 || both.Metadata.Matching != 2 {
		t.Fatal("combined resource filters omitted controlled resources")
	}
	for _, row := range both.Items {
		if row.Name != alpha && row.Name != beta {
			t.Fatal("combined filter admitted an unrelated resource")
		}
	}
	paged := readPage("resource-filter-pagination", "resource", "list", "ignition/schedule", "--filter", "name[sw]=igw-filter-", "--limit", "1", "--offset", "1")
	if len(paged.Items) != 1 || paged.Metadata.Matching != 2 || paged.Metadata.Limit != 1 || paged.Metadata.Offset != 1 {
		t.Fatal("filter interfered with pagination")
	}
	missing := readPage("resource-nonmatch", "resource", "list", "ignition/schedule", "--filter", "name[eq]="+alpha, "--filter", "description[eq]=another value")
	if len(missing.Items) != 0 || missing.Metadata.Matching != 0 {
		t.Fatal("contradictory resource filters matched")
	}
	one(readPage("generic-resource-filter", "api", "request", "GET /data/api/v1/resources/list/ignition/schedule", "--query", "name[eq]="+alpha), alpha)
	preview := ok("generic-filter-preview", "api", "request", "GET /data/api/v1/resources/list/ignition/schedule", "--query", "name[eq]="+alpha, "--dry-run")
	if preview.Outcome != "preview" || requests.Load() != 0 {
		t.Fatal("filter preview sent an operation")
	}

	for _, name := range []string{alpha, beta} {
		body, _ := json.Marshal(map[string]any{"name": name, "description": special, "title": "Query qualification", "enabled": false})
		got := ok("create-project-"+name, "api", "request", "POST /data/api/v1/projects", "--body", string(body), "--yes")
		var ack struct{ Success bool }
		if json.Unmarshal(got.Data, &ack) != nil || !ack.Success {
			t.Fatal("query project setup was not acknowledged")
		}
	}
	project := readPage("project-name-eq", "project", "list", "--filter", "name[eq]="+alpha)
	one(project, alpha)
	if project.Items[0].Enabled == nil || *project.Items[0].Enabled {
		t.Fatal("query project was not disabled")
	}
	projects := readPage("project-multiple-filters", "project", "list", "--filter", "name[sw]=igw-filter-", "--filter", "description[eq]="+special)
	if len(projects.Items) != 2 || projects.Metadata.Matching != 2 {
		t.Fatal("project filters did not preserve exact text")
	}
	for _, row := range projects.Items {
		if (row.Name != alpha && row.Name != beta) || row.Description != special {
			t.Fatal("combined project filter admitted an unrelated item")
		}
	}
	missing = readPage("project-nonmatch", "project", "list", "--filter", "name[eq]=igw-filter-missing")
	if len(missing.Items) != 0 || missing.Metadata.Matching != 0 {
		t.Fatal("nonmatching project filter was ignored")
	}

	logs := readPage("logs-baseline", "logs", "list", "--limit", "50")
	if len(logs.Items) == 0 || logs.Items[0].LoggerName == "" {
		t.Fatal("no log row available for filter verification")
	}
	logger := logs.Items[0].LoggerName
	filtered := readPage("logs-logger-eq", "logs", "list", "--filter", "loggerName[eq]="+logger, "--limit", "10")
	if len(filtered.Items) == 0 {
		t.Fatal("logger filter omitted observed records")
	}
	for _, row := range filtered.Items {
		if row.LoggerName != logger {
			t.Fatal("logger filter admitted another logger")
		}
	}
	missing = readPage("logs-nonmatch", "logs", "list", "--filter", "loggerName[eq]=igw-filter-missing-logger")
	if len(missing.Items) != 0 || missing.Metadata.Matching != 0 {
		t.Fatal("nonmatching logger filter was ignored")
	}

	for _, command := range [][]string{{"resource", "list", "ignition/schedule"}, {"project", "list"}, {"logs", "list"}} {
		got := run(command[0]+"-invalid-operator", append(command, "--filter", "name[unknown]=value")...)
		if got.OK || got.Error == nil || got.Error.Kind != "validation" || got.Error.Code != 2 || requests.Load() != 0 {
			t.Fatal("invalid operator did not fail before dispatch")
		}
		got = run(command[0]+"-duplicate-key", append(command, "--filter", "name[eq]=one", "--filter", "name[eq]=two")...)
		if got.OK || got.Error == nil || got.Error.Code != 2 || requests.Load() != 0 {
			t.Fatal("duplicate filter did not fail before dispatch")
		}
	}
}
