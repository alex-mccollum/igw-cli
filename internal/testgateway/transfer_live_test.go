package testgateway_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/cli"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

type transferResult struct {
	OK       bool            `json:"ok"`
	Outcome  string          `json:"outcome"`
	Data     json.RawMessage `json:"data"`
	Error    *result.Problem `json:"error"`
	Meta     result.Metadata `json:"meta"`
	Artifact *artifact.Info  `json:"artifact"`
}
type transferCheck struct {
	Name              string `json:"name"`
	Outcome           string `json:"outcome"`
	HTTPStatus        int    `json:"httpStatus,omitempty"`
	OperationRequests int64  `json:"operationRequests"`
	ErrorKind         string `json:"errorKind,omitempty"`
	ExitCode          int    `json:"exitCode"`
}

type transferTransport struct {
	base       http.RoundTripper
	operations *atomic.Int64
}

func (t transferTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path != "/openapi.json" {
		t.operations.Add(1)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

func TestLiveProjectTagContract(t *testing.T) {
	testLiveProjectTag(t, false)
}

func TestLiveProjectTagWorkflows(t *testing.T) {
	testLiveProjectTag(t, true)
}

func testLiveProjectTag(t *testing.T, workflows bool) {
	image := os.Getenv("IGW_ACCEPTANCE_TEST_IMAGE")
	if image == "" {
		t.Skip("requires a pinned image and guarded live invocation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	started := time.Now().UTC()
	cfg, err := testgateway.ProfileConfig(image, os.Getenv("IGW_CAPTURE_TEST_DOCKER"), os.Getenv("IGW_TEST_MODULE_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := testgateway.Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Errorf("owned cleanup: %v", err)
		}
	}()
	if _, err := s.WaitOpenAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateModuleProfile(); err != nil {
		t.Fatal(err)
	}
	token, err := s.ProvisionAPIToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, secret, _ := strings.Cut(token, ":")
	cache := t.TempDir()
	dir := t.TempDir()
	if requested := os.Getenv("IGW_TRANSFER_ARTIFACTS"); requested != "" {
		if err := os.Mkdir(requested, 0700); err != nil {
			t.Fatal(err)
		}
		dir = requested
	}
	var checks []transferCheck
	run := func(name string, args ...string) transferResult {
		t.Helper()
		var out, stderr bytes.Buffer
		var requests atomic.Int64
		client := s.HTTPClient()
		client.Transport = transferTransport{base: http.DefaultTransport, operations: &requests}
		app := cli.App{In: strings.NewReader(""), Out: &out, Err: &stderr, CacheDir: cache, HTTP: client, Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) { return config.File{GatewayURL: s.URL, Token: token}, nil }}
		err := app.Run(ctx, append(args, "--json", "--timeout", "90s"))
		if strings.Contains(out.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatal("credential leaked")
		}
		var got transferResult
		if json.Unmarshal(out.Bytes(), &got) != nil || got.OK != (err == nil) {
			t.Fatalf("%s invalid envelope", name)
		}
		status := got.Meta.HTTPStatus
		if got.Error != nil {
			b, _ := json.Marshal(got.Error.Details)
			var details struct {
				HTTPStatus int `json:"httpStatus"`
			}
			_ = json.Unmarshal(b, &details)
			status = details.HTTPStatus
		}
		check := transferCheck{Name: name, Outcome: got.Outcome, HTTPStatus: status, OperationRequests: requests.Load()}
		if got.Error != nil {
			check.ErrorKind, check.ExitCode = got.Error.Kind, got.Error.Code
		}
		checks = append(checks, check)
		t.Logf("%s: %s HTTP=%d", name, got.Outcome, status)
		return got
	}
	ensure := func(name string, got transferResult) transferResult {
		t.Helper()
		if !got.OK {
			t.Fatalf("%s: %v", name, got.Error)
		}
		return got
	}
	confirmed := func(name string, got transferResult) {
		t.Helper()
		ensure(name, got)
		var ack struct{ Success bool }
		if json.Unmarshal(got.Data, &ack) != nil || !ack.Success {
			t.Fatalf("%s not acknowledged", name)
		}
	}
	synced := ensure("catalog", run("catalog", "spec", "sync"))
	discovered := ensure("tag-capabilities", run("tag-capabilities", "api", "capabilities"))
	var capabilities []catalog.CapabilityAssessment
	if json.Unmarshal(discovered.Data, &capabilities) != nil || discovered.Meta.Catalog == nil || synced.Meta.Catalog == nil || discovered.Meta.Catalog.ContractSHA256 != synced.Meta.Catalog.ContractSHA256 {
		t.Fatal("tag capabilities were not derived from the synchronized catalog")
	}
	tagsAvailable, err := reference.TagRoundTripAvailable(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	const sourceName = "igw-transfer-source"
	const targetName = "igw-transfer-copy"
	confirmed("project-create", run("project-create", "api", "request", "POST /data/api/v1/projects", "--body", `{"name":"igw-transfer-source","description":"transfer qualification","title":"Qualification","enabled":false}`, "--yes"))
	source := filepath.Join(dir, "source.zip")
	target := filepath.Join(dir, "copy.zip")
	export := func(step, name, path string) {
		t.Helper()
		got := ensure(step, run(step, "api", "request", "GET /data/api/v1/projects/export/{name}", "--path-param", "name="+name, "--out", path))
		if got.Artifact == nil || got.Artifact.Bytes == 0 {
			t.Fatal("project archive missing")
		}
	}
	export("project-export", sourceName, source)
	preview := ensure("project-preview", run("project-preview", "api", "request", "POST /data/api/v1/projects/import/{name}", "--path-param", "name="+targetName, "--upload", source, "--content-type", "application/zip", "--dry-run"))
	if preview.Outcome != "preview" {
		t.Fatal("import preview sent request")
	}
	absent := run("project-preview-absence", "api", "request", "GET /data/api/v1/projects/find/{name}", "--path-param", "name="+targetName)
	if absent.OK || checks[len(checks)-1].HTTPStatus != 404 {
		t.Fatal("preview changed target project")
	}
	confirmed("project-import", run("project-import", "api", "request", "POST /data/api/v1/projects/import/{name}", "--path-param", "name="+targetName, "--upload", source, "--content-type", "application/zip", "--yes"))
	got := ensure("project-get", run("project-get", "api", "request", "GET /data/api/v1/projects/find/{name}", "--path-param", "name="+targetName))
	var project struct {
		Name, Description, Title string
		Enabled                  bool
	}
	if json.Unmarshal(got.Data, &project) != nil || project.Name != targetName || project.Description != "transfer qualification" || project.Title != "Qualification" || project.Enabled {
		t.Fatal("imported project properties differ")
	}
	export("project-reexport", targetName, target)
	sourceFiles, targetFiles := archiveContents(t, source), archiveContents(t, target)
	t.Logf("project archive entries: %v", sortedKeys(sourceFiles))
	if !reflect.DeepEqual(sourceFiles, targetFiles) {
		t.Fatal("project archive contents changed during round trip")
	}
	duplicate := run("project-existing-refused", "api", "request", "POST /data/api/v1/projects/import/{name}", "--path-param", "name="+targetName, "--upload", source, "--content-type", "application/zip", "--yes")
	if duplicate.OK || checks[len(checks)-1].HTTPStatus != 409 {
		t.Fatal("project import overwrote existing target")
	}
	if tagsAvailable {
		const tags = `{"tags":[{"name":"igwQualification","tagType":"Folder","tags":[{"name":"Counter","tagType":"AtomicTag","valueSource":"memory","dataType":"Int4","value":42}]}]}`
		input := filepath.Join(dir, "tags-input.json")
		if err := os.WriteFile(input, []byte(tags), 0600); err != nil {
			t.Fatal(err)
		}
		importTags := func(step, policy string) transferResult {
			return run(step, "api", "request", "POST /data/api/v1/tags/import", "--query", "provider=default", "--query", "type=json", "--query", "collisionPolicy="+policy, "--upload", input, "--content-type", "application/octet-stream", "--yes")
		}
		good := func(step string, got transferResult) {
			t.Helper()
			ensure(step, got)
			var codes struct {
				SuccessCount int
				FailureCount int
				Failures     []json.RawMessage
			}
			if json.Unmarshal(got.Data, &codes) != nil || codes.SuccessCount < 1 || codes.FailureCount != 0 || len(codes.Failures) != 0 {
				t.Fatalf("%s returned import problems on the fresh test Gateway: %s", step, got.Data)
			}
		}
		good("tag-import", importTags("tag-import", "Abort"))
		exportTags := func(step, path string) {
			t.Helper()
			ensure(step, run(step, "api", "request", "GET /data/api/v1/tags/export", "--query", "provider=default", "--query", "type=json", "--query", "path=igwQualification", "--query", "recursive=true", "--query", "includeUdts=false", "--out", path))
		}
		firstTags := filepath.Join(dir, "tags-export.json")
		exportTags("tag-export", firstTags)
		first, err := os.ReadFile(firstTags)
		if err != nil {
			t.Fatal(err)
		}
		var tree map[string]json.RawMessage
		if json.Unmarshal(first, &tree) != nil {
			t.Fatal("tag export is not JSON")
		}
		t.Logf("tag export root keys: %v", sortedKeys(tree))
		if !hasCounterValue(first, "42") {
			t.Fatal("tag export omitted imported tag or value")
		}
		if err := os.WriteFile(input, []byte(strings.Replace(tags, `42`, `43`, 1)), 0600); err != nil {
			t.Fatal(err)
		}
		good("tag-overwrite", importTags("tag-overwrite", "Overwrite"))
		afterTags := filepath.Join(dir, "tags-after.json")
		exportTags("tag-reexport", afterTags)
		after, err := os.ReadFile(afterTags)
		if err != nil || !hasCounterValue(after, "43") {
			t.Fatal("tag overwrite not observable")
		}
		conflict := ensure("tag-abort-conflict", importTags("tag-abort-conflict", "Abort"))
		var codes struct {
			FailureCount int
			Failures     []json.RawMessage
		}
		if json.Unmarshal(conflict.Data, &codes) != nil || codes.FailureCount < 1 || len(codes.Failures) < 1 {
			t.Fatal("tag Abort did not report duplicate problems")
		}
	} else {
		qualifyUnavailableTags(t, run, dir)
		for _, check := range checks[len(checks)-3:] {
			if check.OperationRequests != 0 || check.HTTPStatus != 0 || check.ErrorKind != "capability" || check.ExitCode != 2 {
				t.Fatal("unavailable tag workflow did not refuse before dispatch")
			}
		}
	}
	if workflows {
		qualifyTransferWorkflows(t, run, dir, sourceName, source, tagsAvailable)
	}
	for _, name := range []string{sourceName, targetName} {
		confirmed("project-delete", run("project-delete-"+name, "api", "request", "DELETE /data/api/v1/projects/{name}", "--path-param", "name="+name, "--query", "confirm=true", "--yes"))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("IGW_TRANSFER_EVIDENCE"); path != "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(executable)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.New()
		_, hashErr := io.Copy(hash, file)
		closeErr := file.Close()
		if hashErr != nil || closeErr != nil {
			t.Fatal("cannot hash test executable")
		}
		receipt := struct {
			Version          int                            `json:"version"`
			Kind             string                         `json:"kind"`
			Image            string                         `json:"image"`
			ImageID          string                         `json:"imageId"`
			Platform         string                         `json:"platform"`
			ModuleInventory  *testgateway.ModuleInventory   `json:"moduleInventory"`
			ModuleWhitelist  []string                       `json:"moduleWhitelist,omitempty"`
			GatewayVersion   string                         `json:"gatewayVersion"`
			TestBinarySHA256 string                         `json:"testBinarySha256"`
			StartedAt        time.Time                      `json:"startedAt"`
			FinishedAt       time.Time                      `json:"finishedAt"`
			Catalog          *catalog.Metadata              `json:"catalog"`
			Capabilities     []catalog.CapabilityAssessment `json:"capabilities"`
			Checks           []transferCheck                `json:"checks"`
			ProjectFiles     []string                       `json:"projectFiles"`
			Cleanup          bool                           `json:"cleanup"`
			Passed           bool                           `json:"passed"`
		}{3, "project-tag-contract", image, s.ImageID, s.Platform, s.ModuleInventory, s.Modules, s.GatewayVersion, hex.EncodeToString(hash.Sum(nil)), started, time.Now().UTC(), synced.Meta.Catalog, capabilities, checks, sortedKeys(sourceFiles), true, true}
		if workflows {
			receipt.Kind = "project-tag-workflows"
		}
		b, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		w, err := artifact.New(path, false)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Abort()
		if _, err := w.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func archiveContents(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	files := map[string]string{}
	for _, entry := range r.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.UncompressedSize64 > 2<<20 {
			t.Fatal("unexpected large qualification archive entry")
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(io.LimitReader(reader, 2<<20+1))
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(entry.Name, ".json") {
			var value any
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			if d.Decode(&value) != nil {
				t.Fatal("invalid project JSON")
			}
			raw, _ = json.Marshal(value)
		}
		hash := sha256.Sum256(raw)
		files[entry.Name] = hex.EncodeToString(hash[:])
	}
	return files
}

func hasCounterValue(raw []byte, want string) bool {
	type tag struct {
		Name  string
		Value json.RawMessage
		Tags  []json.RawMessage
	}
	count := 0
	var walk func(json.RawMessage) bool
	walk = func(raw json.RawMessage) bool {
		var node tag
		if json.Unmarshal(raw, &node) != nil {
			return false
		}
		if node.Name == "Counter" {
			count++
			if string(node.Value) != want {
				return false
			}
		}
		for _, child := range node.Tags {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	return walk(raw) && count == 1
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
