package testgateway_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/nextcli"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

type operationalObservation struct {
	Name       string `json:"name"`
	Outcome    string `json:"outcome"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	State      string `json:"state,omitempty"`
	FileSize   int64  `json:"fileSize,omitempty"`
}
type operationalArtifact struct {
	Name       string   `json:"name"`
	SHA256     string   `json:"sha256"`
	Bytes      int64    `json:"bytes"`
	Format     string   `json:"format"`
	EntryCount int      `json:"entryCount,omitempty"`
	Entries    []string `json:"entries,omitempty"`
}

func TestLiveOperationsContract(t *testing.T) {
	testLiveOperations(t, false)
}

func TestLiveOperationalWorkflows(t *testing.T) {
	testLiveOperations(t, true)
}

func testLiveOperations(t *testing.T, workflows bool) {
	image := os.Getenv("IGW_ACCEPTANCE_TEST_IMAGE")
	if image == "" {
		t.Skip("requires a pinned image and guarded live invocation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	started := time.Now().UTC()
	s, err := testgateway.Start(ctx, testgateway.Config{Image: image, Docker: os.Getenv("IGW_CAPTURE_TEST_DOCKER")})
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
	token, err := s.ProvisionAPIToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, secret, _ := strings.Cut(token, ":")
	cache, dir := t.TempDir(), t.TempDir()
	if requested := os.Getenv("IGW_OPERATIONS_ARTIFACTS"); requested != "" {
		if err := os.Mkdir(requested, 0700); err != nil {
			t.Fatal(err)
		}
		dir = requested
	}
	var checks []operationalObservation
	run := func(name string, args ...string) transferResult {
		t.Helper()
		var out, stderr bytes.Buffer
		app := nextcli.App{In: strings.NewReader(""), Out: &out, Err: &stderr, CacheDir: cache, HTTP: s.HTTPClient(), Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) { return config.File{GatewayURL: s.URL, Token: token}, nil }}
		err := app.Run(ctx, append(args, "--json", "--timeout", "90s"))
		if strings.Contains(out.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatal("credential leaked")
		}
		var got transferResult
		if json.Unmarshal(out.Bytes(), &got) != nil || got.OK != (err == nil) {
			t.Fatalf("%s invalid envelope", name)
		}
		observation := operationalObservation{Name: name, Outcome: got.Outcome, HTTPStatus: got.Meta.HTTPStatus}
		var status struct {
			State    string
			FileSize int64
		}
		_ = json.Unmarshal(got.Data, &status)
		observation.State, observation.FileSize = status.State, status.FileSize
		checks = append(checks, observation)
		t.Logf("%s: %s HTTP=%d state=%s fileSize=%d", name, got.Outcome, observation.HTTPStatus, status.State, status.FileSize)
		if !got.OK {
			t.Fatalf("%s: %v", name, got.Error)
		}
		return got
	}
	synced := run("catalog", "spec", "sync")
	run("gateway-info", "api", "request", "GET /data/api/v1/gateway-info")
	run("pending-restarts", "api", "request", "GET /data/api/v1/restart-tasks/pending")
	logs := run("logs-list", "api", "request", "GET /data/api/v1/logs")
	var list map[string]json.RawMessage
	if json.Unmarshal(logs.Data, &list) != nil || list["items"] == nil {
		t.Fatal("log list shape unsupported")
	}
	t.Logf("log result keys: %v", sortedKeys(list))
	var artifacts []operationalArtifact
	download := func(name, operation, filename string) {
		t.Helper()
		path := filepath.Join(dir, filename)
		got := run(name, "api", "request", operation, "--out", path, "--max-body-bytes", "134217728")
		if got.Artifact == nil || got.Artifact.Bytes == 0 {
			t.Fatal("empty operational artifact")
		}
		details := inspectOperationalArtifact(t, name, path)
		if details.Bytes != got.Artifact.Bytes || details.SHA256 != got.Artifact.SHA256 {
			t.Fatal("artifact receipt differs from file")
		}
		artifacts = append(artifacts, details)
	}
	download("backup-export", "GET /data/api/v1/backup", "gateway.gwbk")
	download("logs-download", "GET /data/api/v1/logs/download", "gateway-logs.bin")
	status := run("bundle-initial-status", "api", "request", "GET /data/api/v1/diagnostics/bundle/status")
	var before struct {
		State    string
		FileSize int64
	}
	if json.Unmarshal(status.Data, &before) != nil || before.State == "" || before.FileSize != 0 {
		t.Fatal("fresh Gateway already has a bundle or unrecognized status")
	}
	preview := run("bundle-preview", "api", "request", "POST /data/api/v1/diagnostics/bundle/generate", "--dry-run")
	if preview.Outcome != "preview" {
		t.Fatal("bundle preview did not remain a preview")
	}
	status = run("bundle-preview-unchanged", "api", "request", "GET /data/api/v1/diagnostics/bundle/status")
	var after struct {
		State    string
		FileSize int64
	}
	if json.Unmarshal(status.Data, &after) != nil || after != before {
		t.Fatal("bundle preview mutated state")
	}
	run("bundle-generate", "api", "request", "POST /data/api/v1/diagnostics/bundle/generate", "--yes")
	// Discover state vocabulary on a fresh Gateway. Readiness for this probe
	// requires an independently downloaded complete archive of the reported size.
	// Production polling must additionally use a qualified terminal state.
	for attempt := 0; ; attempt++ {
		if attempt >= 30 {
			t.Fatal("bundle did not become downloadable within 60 seconds")
		}
		status = run("bundle-status", "api", "request", "GET /data/api/v1/diagnostics/bundle/status")
		if json.Unmarshal(status.Data, &after) != nil || after.State == "" || after.FileSize < 0 {
			t.Fatal("invalid bundle state")
		}
		if after.FileSize > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	download("bundle-download", "GET /data/api/v1/diagnostics/bundle/download", "diagnostics.zip")
	if artifacts[len(artifacts)-1].Bytes != after.FileSize {
		t.Fatal("bundle status size differs from complete download")
	}
	run("bundle-after-download", "api", "request", "GET /data/api/v1/diagnostics/bundle/status")
	if workflows {
		run("bundle-regenerate", "api", "request", "POST /data/api/v1/diagnostics/bundle/generate", "--yes")
		for attempt := 0; ; attempt++ {
			if attempt >= 30 {
				t.Fatal("repeat generation remained pending")
			}
			got := run("bundle-repeat-status", "api", "request", "GET /data/api/v1/diagnostics/bundle/status")
			var status struct {
				State    string
				FileSize int64
			}
			if json.Unmarshal(got.Data, &status) != nil {
				t.Fatal("invalid repeat status")
			}
			if status.State == "Valid" && status.FileSize > 0 {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(2 * time.Second):
			}
		}
		artifacts = append(artifacts, qualifyOperationalWorkflows(t, run, dir)...)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("IGW_OPERATIONS_EVIDENCE"); path != "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(executable)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.New()
		_, hashErr := io.Copy(h, file)
		closeErr := file.Close()
		if hashErr != nil || closeErr != nil {
			t.Fatal("cannot hash executable")
		}
		receipt := struct {
			Version        int                      `json:"version"`
			Kind           string                   `json:"kind"`
			Image          string                   `json:"image"`
			ImageID        string                   `json:"imageId"`
			Platform       string                   `json:"platform"`
			GatewayVersion string                   `json:"gatewayVersion"`
			BinarySHA256   string                   `json:"testBinarySha256"`
			StartedAt      time.Time                `json:"startedAt"`
			FinishedAt     time.Time                `json:"finishedAt"`
			Catalog        *catalog.Metadata        `json:"catalog"`
			Checks         []operationalObservation `json:"checks"`
			Artifacts      []operationalArtifact    `json:"artifacts"`
			Cleanup        bool                     `json:"cleanup"`
			Passed         bool                     `json:"passed"`
		}{2, "operations-contract", image, s.ImageID, s.Platform, s.GatewayVersion, hex.EncodeToString(h.Sum(nil)), started, time.Now().UTC(), synced.Meta.Catalog, checks, artifacts, true, true}
		if workflows {
			receipt.Kind = "operational-workflows"
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

func inspectOperationalArtifact(t *testing.T, name, path string) operationalArtifact {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() <= 0 || info.Size() > 128<<20 {
		t.Fatal("unexpected qualification artifact size")
	}
	result := operationalArtifact{Name: name, Bytes: info.Size()}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		t.Fatal(err)
	}
	result.SHA256 = hex.EncodeToString(h.Sum(nil))
	var magic [16]byte
	if _, err := file.ReadAt(magic[:], 0); err != nil {
		t.Fatal(err)
	}
	if string(magic[:]) == "SQLite format 3\x00" {
		result.Format = "sqlite3"
		return result
	}
	z, err := zip.NewReader(file, info.Size())
	if err != nil || len(z.File) == 0 || len(z.File) > 10000 {
		t.Fatal("unsupported operational artifact format")
	}
	result.Format = "zip"
	var expanded int64
	for _, entry := range z.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		expanded += int64(entry.UncompressedSize64)
		if entry.UncompressedSize64 > 128<<20 || expanded > 256<<20 {
			t.Fatal("unexpected expanded artifact size")
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		n, readErr := io.Copy(io.Discard, io.LimitReader(reader, int64(entry.UncompressedSize64)+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || n != int64(entry.UncompressedSize64) {
			t.Fatal("incomplete operational archive")
		}
		result.EntryCount++
		if result.EntryCount <= 16 {
			result.Entries = append(result.Entries, entry.Name)
		}
	}
	if result.EntryCount > 16 {
		result.Entries = nil
	}
	t.Logf("%s: %s bytes=%d entries=%d", name, result.Format, result.Bytes, result.EntryCount)
	return result
}
