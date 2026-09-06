package testgateway_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/cli"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

type inputWire struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	ContentType   string `json:"contentType,omitempty"`
	ContentLength int64  `json:"contentLength"`
	BodyBytes     int64  `json:"bodyBytes"`
	BodySHA256    string `json:"bodySha256"`
}

// Observe the bytes net/http consumes without buffering or rewriting them.
// Snapshotting is safe even when the transport closes a body asynchronously.
type inputBodyReader struct {
	io.ReadCloser
	mu   sync.Mutex
	hash hash.Hash
	size int64
}

func (r *inputBodyReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.mu.Lock()
	_, _ = r.hash.Write(p[:n])
	r.size += int64(n)
	r.mu.Unlock()
	return n, err
}

type inputObserver struct {
	base              http.RoundTripper
	mu                sync.Mutex
	wire              []inputWire
	body              []*inputBodyReader
	catalogRequests   int
	confirmedRestarts int
}

func (o *inputObserver) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/openapi.json" {
		o.mu.Lock()
		o.catalogRequests++
		o.mu.Unlock()
	} else {
		body := &inputBodyReader{ReadCloser: r.Body, hash: sha256.New()}
		o.mu.Lock()
		if r.Method == "POST" && r.URL.Path == "/data/api/v1/restart-tasks/restart" && r.URL.RawQuery == "confirm=true" {
			o.confirmedRestarts++
		}
		o.wire = append(o.wire, inputWire{Method: r.Method, Path: r.URL.EscapedPath(), ContentType: r.Header.Get("Content-Type"), ContentLength: r.ContentLength})
		o.body = append(o.body, body)
		o.mu.Unlock()
		// Preserve the sentinel: wrapping NoBody would turn a known empty
		// representation into an unknown-length/chunked request in net/http.
		if r.Body != nil && r.Body != http.NoBody {
			r = r.Clone(r.Context())
			r.Body = body
		}
	}
	base := o.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

func (o *inputObserver) snapshot() []inputWire {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := append([]inputWire(nil), o.wire...)
	for i, body := range o.body {
		body.mu.Lock()
		out[i].BodyBytes, out[i].BodySHA256 = body.size, hex.EncodeToString(body.hash.Sum(nil))
		body.mu.Unlock()
	}
	return out
}

type inputCheck struct {
	transferCheck
	CatalogRequests int                             `json:"catalogRequests,omitempty"`
	Batch           *inputBatchEvidence             `json:"batch,omitempty"`
	Validation      string                          `json:"validation,omitempty"`
	Wire            []inputWire                     `json:"wire,omitempty"`
	Preview         *execute.Preview                `json:"preview,omitempty"`
	Artifact        *inputArtifact                  `json:"artifact,omitempty"`
	Restart         *inputRestartEvidence           `json:"restart,omitempty"`
	Resource        *inputResourceEvidence          `json:"resource,omitempty"`
	Process         *testgateway.ProcessObservation `json:"process,omitempty"`
}

type inputArtifact struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type inputReceipt struct {
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
	Checks           []inputCheck                 `json:"checks"`
	Cleanup          bool                         `json:"cleanup"`
	Passed           bool                         `json:"passed"`
}

type inputSuite struct {
	t           *testing.T
	ctx         context.Context
	session     *testgateway.Session
	dir         string
	cache       string
	token       string
	receipt     inputReceipt
	completed   bool
	receiptName string
}

func beginInputSuite(t *testing.T) *inputSuite {
	t.Helper()
	return beginObservedSuite(t, "IGW_INPUT_EVIDENCE_DIR", "request-body-inputs", "body-inputs.json")
}

func beginObservedSuite(t *testing.T, directoryVariable, kind, receiptName string) *inputSuite {
	t.Helper()
	image := os.Getenv("IGW_ACCEPTANCE_TEST_IMAGE")
	if image == "" {
		t.Skip("requires a pinned image and guarded live invocation")
	}
	dir := os.Getenv(directoryVariable)
	if dir == "" {
		t.Fatalf("requires a new %s", directoryVariable)
	}
	cfg, err := testgateway.ProfileConfig(image, os.Getenv("IGW_CAPTURE_TEST_DOCKER"), os.Getenv("IGW_TEST_MODULE_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	s := &inputSuite{t: t, ctx: ctx, dir: dir, cache: t.TempDir(), receiptName: receiptName, receipt: inputReceipt{Version: 1, Kind: kind, Image: image, StartedAt: time.Now().UTC(), Checks: []inputCheck{}}}
	t.Cleanup(func() { s.finish(); cancel() })
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	_, hashErr := io.Copy(h, f)
	closeErr := f.Close()
	if hashErr != nil || closeErr != nil {
		t.Fatal("cannot hash body acceptance executable")
	}
	s.receipt.TestBinarySHA256 = hex.EncodeToString(h.Sum(nil))
	s.session, err = testgateway.Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.session.WaitOpenAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.session.ValidateModuleProfile(); err != nil {
		t.Fatal(err)
	}
	s.token, err = s.session.ProvisionAPIToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	synced := s.run("catalog", nil, "spec", "sync")
	if !synced.OK || synced.Meta.Catalog == nil || synced.Meta.Target == nil {
		t.Fatal("input catalog provenance missing")
	}
	s.receipt.Catalog = synced.Meta.Catalog
	snapshot, err := (catalog.Store{Dir: s.cache}).Load(*synced.Meta.Target)
	if err != nil {
		t.Fatal(err)
	}
	raw := snapshot.Catalog.Raw()
	if snapshot.Metadata.RawSHA256 != s.receipt.Catalog.RawSHA256 {
		snapshot.Close()
		t.Fatal("input catalog export changed snapshot")
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
	s.receipt.OpenAPI = &info
	return s
}

func (s *inputSuite) finish() {
	if s.session != nil {
		if err := s.session.Close(); err != nil {
			s.t.Errorf("owned input Gateway cleanup: %v", err)
		} else {
			s.receipt.Cleanup = true
		}
		s.receipt.ImageID, s.receipt.Platform, s.receipt.GatewayVersion = s.session.ImageID, s.session.Platform, s.session.GatewayVersion
		s.receipt.ModuleInventory, s.receipt.ModuleWhitelist = s.session.ModuleInventory, s.session.Modules
	}
	s.receipt.FinishedAt = time.Now().UTC()
	s.receipt.Passed = s.completed && !s.t.Failed() && s.receipt.Cleanup
	b, err := json.MarshalIndent(s.receipt, "", "  ")
	if err != nil {
		s.t.Error(err)
		return
	}
	w, err := artifact.New(filepath.Join(s.dir, s.receiptName), false)
	if err != nil {
		s.t.Error(err)
		return
	}
	defer w.Abort()
	if _, err := w.Write(append(b, '\n')); err != nil {
		s.t.Error(err)
		return
	}
	if _, err := w.Commit(); err != nil {
		s.t.Error(err)
	}
}

func (s *inputSuite) run(name string, input io.Reader, args ...string) transferResult {
	s.t.Helper()
	client := s.session.HTTPClient()
	observer := &inputObserver{base: client.Transport}
	client.Transport = observer
	var out, stderr bytes.Buffer
	app := cli.App{In: input, Out: &out, Err: &stderr, CacheDir: s.cache, HTTP: client, Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) { return config.File{GatewayURL: s.session.URL, Token: s.token}, nil }}
	err := app.Run(s.ctx, append(args, "--json", "--timeout", "90s"))
	_, secret, _ := strings.Cut(s.token, ":")
	if strings.Contains(out.String()+stderr.String(), s.token) || (secret != "" && strings.Contains(out.String()+stderr.String(), secret)) {
		s.t.Fatal("input CLI exposed its credential")
	}
	var got transferResult
	if json.Unmarshal(out.Bytes(), &got) != nil || got.OK != (err == nil) {
		s.t.Fatalf("%s invalid CLI envelope", name)
	}
	check := inputCheck{transferCheck: transferCheck{Name: name, Outcome: got.Outcome, HTTPStatus: inputHTTPStatus(got)}, Validation: got.Meta.Validation, Wire: observer.snapshot()}
	observer.mu.Lock()
	check.CatalogRequests = observer.catalogRequests
	observer.mu.Unlock()
	check.OperationRequests = int64(len(check.Wire))
	if got.Error != nil {
		check.ErrorKind, check.ExitCode = got.Error.Kind, got.Error.Code
	}
	batch := len(args) >= 2 && args[0] == "api" && args[1] == "batch"
	restart := len(args) >= 2 && args[0] == "gateway" && args[1] == "restart"
	resourceChange := len(args) >= 2 && args[0] == "resource" && (args[1] == "create" || args[1] == "update" || args[1] == "delete")
	if batch {
		check.Batch = batchEvidence(got.Data)
	} else if restart {
		check.Restart = restartEvidence(got.Data)
		if check.Restart != nil {
			observer.mu.Lock()
			check.Restart.ConfirmedRequests = observer.confirmedRestarts
			observer.mu.Unlock()
		}
	} else if resourceChange {
		check.Resource = resourceEvidence(got.Data)
	} else if got.Outcome == "preview" {
		var preview execute.Preview
		if json.Unmarshal(got.Data, &preview) != nil {
			s.t.Fatalf("%s invalid preview envelope", name)
		}
		check.Preview = &preview
	}
	if got.Artifact != nil {
		check.Artifact = &inputArtifact{Bytes: got.Artifact.Bytes, SHA256: got.Artifact.SHA256}
	}
	s.receipt.Checks = append(s.receipt.Checks, check)
	s.t.Logf("%s: %s HTTP=%d requests=%d", name, check.Outcome, check.HTTPStatus, check.OperationRequests)
	if got.Meta.Catalog != nil && s.receipt.Catalog != nil && (got.Meta.Catalog.ContractSHA256 != s.receipt.Catalog.ContractSHA256 || got.Meta.Catalog.ParserVersion != catalog.ParserVersion) {
		s.t.Fatal("input invocation changed the captured contract or parser")
	}
	return got
}

func inputHTTPStatus(got transferResult) int {
	if got.Meta.HTTPStatus != 0 {
		return got.Meta.HTTPStatus
	}
	if got.Error != nil {
		b, _ := json.Marshal(got.Error.Details)
		var detail struct{ HTTPStatus int }
		_ = json.Unmarshal(b, &detail)
		return detail.HTTPStatus
	}
	return 0
}

func (s *inputSuite) last() inputCheck { return s.receipt.Checks[len(s.receipt.Checks)-1] }

func (s *inputSuite) require(got transferResult, requests int64) {
	s.t.Helper()
	if !got.OK || s.last().OperationRequests != requests {
		s.t.Fatalf("%s failed: HTTP=%d kind=%s requests=%d", s.last().Name, s.last().HTTPStatus, s.last().ErrorKind, s.last().OperationRequests)
	}
}

func (s *inputSuite) refused(got transferResult) {
	s.t.Helper()
	if got.OK || got.Error == nil || got.Error.Code != 2 || s.last().OperationRequests != 0 {
		s.t.Fatalf("%s was not refused before dispatch", s.last().Name)
	}
}

func inputDigest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Structural JWE acceptance, not decryption or cryptographic validation.
func inputJWE(raw []byte, empty bool) bool {
	var fields map[string]string
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	for _, name := range []string{"protected", "iv", "ciphertext", "tag"} {
		value, present := fields[name]
		if !present || value == "" && (name != "ciphertext" || !empty) {
			return false
		}
		if _, err := base64.RawURLEncoding.DecodeString(value); err != nil {
			return false
		}
	}
	header, err := base64.RawURLEncoding.DecodeString(fields["protected"])
	var protected map[string]any
	if err != nil || json.Unmarshal(header, &protected) != nil {
		return false
	}
	enc, _ := protected["enc"].(string)
	return enc != ""
}

func TestLiveBodyInputs(t *testing.T) {
	s := beginInputSuite(t)
	const encrypt = "POST /data/api/v1/encryption/encrypt"
	text := []byte(" \tigw public input fixture + & = 日本\n")
	binary := bytes.Repeat([]byte{0, 255, 1, 128, 10, 13, 44, 61}, 8192)
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	textFile, binaryFile, emptyFile := write("text.txt", text), write("binary.bin", binary), write("empty.bin", nil)
	for _, tt := range []struct {
		name, media, coverage string
		args                  []string
		stdin, data           []byte
	}{
		{"text-literal", "text/plain", catalog.ValidationSchema, []string{"--body", string(text)}, nil, text},
		{"text-file", "text/plain; charset=utf-8", catalog.ValidationSchema, []string{"--body", "@" + textFile}, nil, text},
		{"text-stdin", "text/plain", catalog.ValidationSchema, []string{"--body", "-"}, text, text},
		{"text-empty-literal", "text/plain", catalog.ValidationSchema, []string{"--body", ""}, nil, nil},
		{"text-empty-file", "text/plain", catalog.ValidationSchema, []string{"--body", "@" + emptyFile}, nil, nil},
		{"text-empty-stdin", "text/plain", catalog.ValidationSchema, []string{"--body", "-"}, nil, nil},
		{"binary-body", "application/octet-stream", catalog.ValidationTransport, []string{"--body", "@" + binaryFile}, nil, binary},
		{"binary-upload", "application/octet-stream", catalog.ValidationTransport, []string{"--upload", binaryFile}, nil, binary},
		{"binary-empty-upload", "application/octet-stream", catalog.ValidationTransport, []string{"--upload", emptyFile}, nil, nil},
	} {
		args := append([]string{"api", "request", encrypt, "--content-type", tt.media}, tt.args...)
		got := s.run(tt.name+"-preview", bytes.NewReader(tt.stdin), append(args, "--dry-run")...)
		s.require(got, 0)
		var preview execute.Preview
		if json.Unmarshal(got.Data, &preview) != nil || got.Outcome != "preview" || !preview.BodyPresent || preview.BodyBytes != int64(len(tt.data)) || preview.BodySHA256 != inputDigest(tt.data) || got.Meta.Validation != tt.coverage {
			t.Fatalf("%s preview did not bind the supplied body", tt.name)
		}
		got = s.run(tt.name, bytes.NewReader(tt.stdin), append(args, "--yes")...)
		s.require(got, 1)
		wire := s.last().Wire[0]
		if got.Meta.Validation != tt.coverage || wire.BodyBytes != int64(len(tt.data)) || wire.ContentLength != int64(len(tt.data)) || wire.BodySHA256 != inputDigest(tt.data) || wire.ContentType != tt.media || !inputJWE(got.Data, len(tt.data) == 0) {
			t.Fatalf("%s payload, media, coverage, or encryption response differs", tt.name)
		}
	}
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"missing-body", []string{"--content-type", "text/plain"}},
		{"undeclared-json", []string{"--content-type", "application/json", "--body", `{}`}},
		{"invalid-utf8", []string{"--content-type", "text/plain", "--body", "@" + binaryFile}},
		{"unsupported-charset", []string{"--content-type", "text/plain; charset=iso-8859-1", "--body", "text"}},
		{"unsupported-text-stream", []string{"--content-type", "text/plain", "--upload", textFile}},
	} {
		for _, mode := range []string{"--dry-run", "--yes"} {
			args := append([]string{"api", "request", encrypt, mode}, tt.args...)
			s.refused(s.run(tt.name+mode, nil, args...))
		}
	}
	s.multipart(dir, binaryFile, textFile, emptyFile, binary, text)
	s.completed = true
}

func (s *inputSuite) multipart(dir, binaryFile, textFile, emptyFile string, binary, text []byte) {
	t := s.t
	const bulk = "PUT /data/api/v1/resources/datafile/ignition/translations"
	const single = "/data/api/v1/resources/datafile/ignition/translations/{filename}"
	const singleton = "GET /data/api/v1/resources/singleton/ignition/translations"
	manifest := []map[string]string{
		{"name": "files", "file": binaryFile, "filename": "igw-input-binary.bin"},
		{"name": "files", "file": textFile, "filename": "igw-input-text.txt", "contentType": "text/plain; charset=utf-8"},
		{"name": "files", "file": emptyFile, "filename": "igw-input-empty.bin"},
	}
	encoded, _ := json.Marshal(manifest)
	if strings.HasPrefix(s.session.GatewayVersion, "8.3.0 ") {
		for _, mode := range []string{"--dry-run", "--yes"} {
			s.refused(s.run("bulk-unavailable"+mode, nil, "api", "request", bulk, "--query", "signature=unavailable", "--multipart", string(encoded), mode))
		}
		return
	}
	before := s.run("translations-before", nil, "api", "request", singleton, "--query", "collection=core")
	created := inputHTTPStatus(before) == 404
	if created {
		got := s.run("translations-create", nil, "api", "request", "POST /data/api/v1/resources/ignition/translations", "--body", `[{"collection":"core","config":{"caseInsensitive":false,"ignoreWhitespace":false,"ignorePunctuation":false,"ignoreTags":false,"terms":{}}}]`, "--yes")
		s.require(got, 1)
	} else {
		s.require(before, 1)
	}
	signature := func(name string) string {
		t.Helper()
		got := s.run(name, nil, "api", "request", singleton, "--query", "collection=core")
		s.require(got, 1)
		var resource struct{ Signature, Collection, Type string }
		if json.Unmarshal(got.Data, &resource) != nil || resource.Signature == "" || resource.Collection != "core" || resource.Type != "ignition/translations" {
			t.Fatal("unverified translations resource identity")
		}
		return resource.Signature
	}
	current := signature("translations-signature")
	files := []struct {
		name string
		data []byte
	}{{"igw-input-binary.bin", binary}, {"igw-input-text.txt", text}, {"igw-input-empty.bin", nil}}
	absent := func(stage string) {
		t.Helper()
		for _, file := range files {
			got := s.run(stage+"-"+file.name, nil, "api", "request", "GET "+single, "--path-param", "filename="+file.name, "--query", "collection=core")
			if got.OK || got.Error == nil || got.Error.Code != 7 || inputHTTPStatus(got) != 404 || s.last().OperationRequests != 1 {
				t.Fatal("qualification file is unexpectedly present")
			}
		}
	}
	absent("files-before")
	args := []string{"api", "request", bulk, "--query", "collection=core", "--query", "signature=" + current, "--multipart", string(encoded)}
	got := s.run("multipart-preview", nil, append(args, "--dry-run")...)
	s.require(got, 0)
	var preview execute.Preview
	if json.Unmarshal(got.Data, &preview) != nil || !preview.BodyPresent || len(preview.Parts) != len(files) || got.Meta.Validation != catalog.ValidationTransport {
		t.Fatal("multipart preview is missing its part identities")
	}
	for i, file := range files {
		part := preview.Parts[i]
		if part.Name != "files" || part.Filename != file.name || part.Bytes != int64(len(file.data)) || part.SHA256 != inputDigest(file.data) {
			t.Fatal("multipart preview changed a part")
		}
	}
	absent("files-after-preview")
	got = s.run("multipart-create", nil, append(args, "--yes")...)
	s.require(got, 1)
	if got.Meta.Validation != catalog.ValidationTransport || !strings.HasPrefix(s.last().Wire[0].ContentType, "multipart/form-data; boundary=") {
		t.Fatal("multipart execution changed coverage or media")
	}
	readFiles := func(stage string) {
		t.Helper()
		for _, file := range files {
			path := filepath.Join(dir, stage+"-"+file.name)
			got := s.run(stage+"-"+file.name, nil, "api", "request", "GET "+single, "--path-param", "filename="+file.name, "--query", "collection=core", "--out", path)
			s.require(got, 1)
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, file.data) || got.Artifact == nil || got.Artifact.Bytes != int64(len(file.data)) || got.Artifact.SHA256 != inputDigest(file.data) {
				t.Fatal("stored file differs from the multipart input")
			}
		}
	}
	readFiles("files-created")
	updated := signature("translations-after-create")
	if updated == current {
		t.Fatal("multipart write did not change resource signature")
	}
	// Review-bound signatures are sent unchanged. A stale write must not be
	// reported as a successful resource update or alter the independently read files.
	got = s.run("multipart-stale-signature", nil, append(args, "--yes")...)
	var acknowledgement struct{ Success *bool }
	_ = json.Unmarshal(got.Data, &acknowledgement)
	if s.last().OperationRequests != 1 || got.OK && (acknowledgement.Success == nil || *acknowledgement.Success) || got.Error != nil && got.Error.Code != 7 {
		t.Fatal("stale multipart signature was not rejected by the Gateway")
	}
	if signature("translations-after-stale") != updated {
		t.Fatal("stale multipart write changed resource state")
	}
	readFiles("files-after-stale")
	got = s.run("single-file-overwrite", nil, "api", "request", "PUT "+single, "--path-param", "filename="+files[0].name, "--query", "collection=core", "--query", "signature="+updated, "--upload", textFile, "--content-type", "application/octet-stream", "--yes")
	s.require(got, 1)
	if got.Meta.Validation != catalog.ValidationTransport || s.last().Wire[0].BodySHA256 != inputDigest(text) {
		t.Fatal("single-file opaque upload changed its payload")
	}
	files[0].data = text
	readFiles("files-overwritten")
	for _, file := range files {
		current = signature("signature-delete-" + file.name)
		got := s.run("delete-"+file.name, nil, "api", "request", "DELETE "+single, "--path-param", "filename="+file.name, "--query", "collection=core", "--query", "signature="+current, "--yes")
		s.require(got, 1)
	}
	absent("files-after-delete")
	if created {
		current = signature("signature-delete-translations")
		got := s.run("translations-delete", nil, "api", "request", "DELETE /data/api/v1/resources/ignition/translations/{signature}", "--path-param", "signature="+current, "--query", "collection=core", "--yes")
		s.require(got, 1)
		if got := s.run("translations-after-delete", nil, "api", "request", singleton, "--query", "collection=core"); inputHTTPStatus(got) != 404 {
			t.Fatal("created translations resource remains after cleanup")
		}
	} else {
		after := s.run("translations-after-files", nil, "api", "request", singleton, "--query", "collection=core")
		s.require(after, 1)
		var previous, restored map[string]json.RawMessage
		if json.Unmarshal(before.Data, &previous) != nil || json.Unmarshal(after.Data, &restored) != nil {
			t.Fatal("cannot compare original translations configuration")
		}
		for _, key := range []string{"config", "backupConfig", "enabled", "description"} {
			if !jsonvalue.Equivalent(previous[key], restored[key], false) {
				t.Fatal("multipart qualification changed original configuration")
			}
		}
	}
}
