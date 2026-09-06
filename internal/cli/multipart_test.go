package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const multipartSpec = `{"openapi":"3.1.0","info":{"title":"Synthetic multipart CLI","version":"test"},"paths":{"/upload":{"put":{"requestBody":{"required":true,"content":{"multipart/form-data":{}}},"responses":{"200":{"description":"OK"}}}}}}`

func TestMultipartCLIStreamsLargeOrderedParts(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	const size int64 = 40 << 20
	path := filepath.Join(t.TempDir(), "private-local-name.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	f.Close()
	var writes atomic.Int32
	var observed atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, multipartSpec)
			return
		}
		writes.Add(1)
		if r.Method != "PUT" || r.URL.Path != "/proxy/upload" || r.Header.Get("X-Ignition-API-Token") != "private-token" || r.ContentLength < size {
			t.Error("multipart route, authorization, or Content-Length changed")
		}
		media, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "multipart/form-data" {
			t.Error("invalid multipart Content-Type")
			return
		}
		reader := multipart.NewReader(r.Body, parameters["boundary"])
		var parts []artifact.MultipartPartInfo
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error("multipart framing was incomplete")
				return
			}
			hash := sha256.New()
			n, err := io.Copy(hash, part)
			if err != nil {
				t.Error("multipart content incomplete")
				return
			}
			parts = append(parts, artifact.MultipartPartInfo{Name: part.FormName(), Filename: part.FileName(), ContentType: part.Header.Get("Content-Type"), Bytes: n, SHA256: hex.EncodeToString(hash.Sum(nil))})
		}
		observed.Store(parts)
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	text, empty := " private field + & 日本 \n", ""
	parts := []artifact.MultipartPart{
		{Name: "field", Text: &text},
		{Name: "files", File: path, Filename: "upload.bin", ContentType: "application/test"},
		{Name: "field", Text: &empty},
	}
	raw, _ := json.Marshal(parts)
	manifest := filepath.Join(t.TempDir(), "parts.json")
	if err := os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	app, out, stderr := testApp(t, srv)
	args := []string{"api", "request", "PUT /upload", "--multipart", "@" + manifest, "--json"}
	if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), path) || strings.Contains(out.String(), "private field") || strings.Contains(out.String(), "private-token") || stderr.Len() != 0 {
		t.Fatal("multipart preview leaked a source path or value")
	}
	preview := decodeResult(t, out)
	var details execute.Preview
	raw, _ = json.Marshal(preview.Data)
	if json.Unmarshal(raw, &details) != nil || preview.Outcome != "preview" || preview.Meta.Validation != "declared_transport" || details.BodyBytes <= size || len(details.Parts) != 3 || details.Parts[1].Bytes != size || details.Parts[2].Bytes != 0 || writes.Load() != 0 {
		t.Fatal("preview lacks exact multipart metadata or sent the operation")
	}
	if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	if !got.OK || got.Meta.Validation != "declared_transport" || writes.Load() != 1 || !reflect.DeepEqual(details.Parts, observed.Load()) {
		t.Fatal("multipart execution changed reviewed part bytes, ordering, or headers")
	}
	entries, _ := os.ReadDir(os.TempDir())
	if len(entries) != 0 {
		t.Fatal("multipart CLI leaked a spool")
	}
}

func TestMultipartCLIRefusesInvalidInputsBeforeDispatch(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "private-source")
	if err := os.WriteFile(path, []byte("private-file-content"), 0600); err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, strings.Replace(multipartSpec, `"multipart/form-data":{}`, `"multipart/form-data":{"schema":{"type":"object","required":["requiredField"]}}`, 1))
			return
		}
		writes.Add(1)
	}))
	defer srv.Close()
	for _, flags := range [][]string{
		{"--form-field", "field=value"}, // Declared schema cannot be bypassed by streaming.
		{"--form-file", "file=" + path},
		{"--multipart", ""},
		{"--multipart", `[]`},
		{"--multipart", `[{"name":"field","text":"private-text","file":""}]`},
		{"--multipart", `[{"name":"field","text":"first","text":"private-text"}]`},
		{"--multipart", `[{"name":"field","Text":"private-text"}]`},
		{"--multipart", `[{"name":"field","text":null}]`},
		{"--multipart", `[{"name":"field","text":"\ud800"}]`},
		{"--multipart", `[{"name":"field","text":"\udc00"}]`},
		{"--multipart", `[{"name":"field","text":"` + "\xff" + `"}]`},
		{"--multipart", `[{"name":"field","text":"private-text","filename":"misleading"}]`},
		{"--form-field", "field=value", "--body", "body"},
		{"--form-field", "field=value", "--upload", path},
		{"--form-file", "file=" + path, "--content-type", "multipart/form-data"},
		{"--multipart", `[{"name":"field","text":"text"}]`, "--form-field", "field=value"},
		{"--form-field", "field"},
		{"--form-file", "file="},
		{"--form-field", "field=value", "--max-upload-bytes", "1"},
		{"--form-field", "field=value", "--max-upload-bytes", "0"},
	} {
		for _, mode := range []string{"--dry-run", "--yes"} {
			app, out, stderr := testApp(t, srv)
			args := append([]string{"api", "request", "PUT /upload", "--json", mode}, flags...)
			err := app.Run(context.Background(), args)
			if strings.Contains(out.String(), "private-text") || strings.Contains(out.String(), path) || strings.Contains(out.String(), "private-token") || strings.Contains(out.String(), "private-file-content") || stderr.Len() != 0 {
				t.Fatal("multipart refusal exposed input or credentials")
			}
			got := decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || got.OK || got.Error == nil || got.Error.Code != 2 || writes.Load() != 0 {
				t.Fatalf("invalid multipart input accepted or dispatched: %+v %v", got, err)
			}
			entries, _ := os.ReadDir(os.TempDir())
			if len(entries) != 0 {
				t.Fatal("multipart refusal leaked a snapshot")
			}
		}
	}
}

func TestMultipartCLIShorthandsAndRaw(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(path, []byte("binary"), 0600); err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/openapi.json" {
			t.Error("explicit raw multipart unexpectedly requested a catalog")
			return
		}
		writes.Add(1)
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		reader := multipart.NewReader(r.Body, params["boundary"])
		for n, expected := range []string{"@literal-file-is-text", "", "binary", "binary"} {
			part, err := reader.NextRawPart()
			if err != nil {
				t.Error(err)
				return
			}
			body, _ := io.ReadAll(part)
			if string(body) != expected || n < 2 && part.FileName() != "" || n >= 2 && part.FileName() != "source.bin" {
				t.Error("shorthand reordered parts or reinterpreted a literal field")
			}
		}
		if _, err := reader.NextRawPart(); err != io.EOF {
			t.Error("unexpected multipart ending")
		}
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	args := []string{"api", "raw", "--method", "PUT", "--path", "/upload", "--form-field", "name=@literal-file-is-text", "--form-file", "files=" + path, "--form-field", "name=", "--form-file", "files=" + path, "--json"}
	if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
		t.Fatal(err)
	}
	if got := decodeResult(t, out); got.Meta.Validation != "not_requested" || writes.Load() != 0 {
		t.Fatal("raw multipart preview has validation claims or dispatched")
	}
	if err := app.Run(context.Background(), args); igwerr.ExitCode(err) != 2 || writes.Load() != 0 {
		t.Fatal("multipart write did not require confirmation")
	}
	decodeResult(t, out)
	if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
		t.Fatal(err)
	}
	if got := decodeResult(t, out); !got.OK || got.Meta.Validation != "not_requested" || writes.Load() != 1 {
		t.Fatal("raw multipart failed or claimed schema validation")
	}
}

func TestMultipartManifestRetainsUnicodeAndLiteralEscapes(t *testing.T) {
	parts, err := decodeMultipartParts([]byte(`[{"name":"field","text":"\ud83d\ude00\ufffd\\ud800\/"}]`))
	if err != nil || len(parts) != 1 || *parts[0].Text != "😀�\\ud800/" {
		t.Fatal("valid JSON Unicode or literal escapes were changed")
	}
}

func TestMultipartInputSchemaIsDiscoverableOffline(t *testing.T) {
	app, out, _ := testApp(t, nil)
	if err := app.Run(context.Background(), []string{"schema", "--json"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(decodeResult(t, out).Data)
	var root commandInfo
	if json.Unmarshal(raw, &root) != nil {
		t.Fatal("command schema was not structured")
	}
	found := 0
	for _, group := range root.Commands {
		if group.Name != "api" {
			continue
		}
		for _, command := range group.Commands {
			if command.Name != "request" && command.Name != "raw" {
				continue
			}
			for _, flag := range command.Flags {
				if flag.Name == "multipart" {
					var schema map[string]any
					if json.Unmarshal(flag.InputSchema, &schema) != nil || schema["type"] != "array" || schema["maxItems"] != float64(artifact.MaxMultipartParts) {
						t.Fatal("multipart input schema missing or incorrect")
					}
					found++
				}
			}
		}
	}
	if found != 2 {
		t.Fatal("both request commands must expose the multipart contract")
	}
}
