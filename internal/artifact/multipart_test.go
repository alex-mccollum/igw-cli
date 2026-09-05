package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestMultipartSnapshotPreservesOrderedParts(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "local-private-name.bin")
	original := []byte("\x00\xffbinary\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	text, empty := " exact + & 日本 \n", ""
	parts := []MultipartPart{
		{Name: `same"\日本`, Text: &text},
		{Name: "files", File: path, Filename: `送信".bin`, ContentType: "application/test"},
		{Name: `same"\日本`, Text: &empty},
		{Name: "files", File: path},
	}
	u, err := SnapshotMultipart(context.Background(), parts, 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	spool := u.file.Name()
	stat, err := u.file.Stat()
	if err != nil || runtime.GOOS != "windows" && stat.Mode().Perm()&0077 != 0 {
		t.Fatal("multipart snapshot is not private")
	}
	if err := os.WriteFile(path, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := u.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if int64(len(raw)) != u.Bytes() || hex.EncodeToString(sum[:]) != u.SHA256() || strings.Contains(string(raw), filepath.Dir(path)) {
		t.Fatal("wire bytes differ from snapshot identity or expose a local directory")
	}
	media, parameters, err := mime.ParseMediaType(u.ContentType())
	if err != nil || media != "multipart/form-data" {
		t.Fatal("missing multipart framing")
	}
	parsed := multipart.NewReader(strings.NewReader(string(raw)), parameters["boundary"])
	info := u.Parts()
	wantBodies := []string{text, string(original), empty, string(original)}
	for n, expected := range info {
		part, err := parsed.NextRawPart()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(part)
		sum := sha256.Sum256(body)
		if err != nil || string(body) != wantBodies[n] || part.FormName() != expected.Name || part.FileName() != expected.Filename || part.Header.Get("Content-Type") != expected.ContentType || expected.Bytes != int64(len(body)) || expected.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("part %d changed values, ordering, headers, or identity", n)
		}
		if len(part.Header) != 2 || strings.Contains(part.Header.Get("Content-Disposition"), "filename*=") {
			t.Fatal("unexpected part headers or RFC5987 filename encoding")
		}
	}
	if _, err := parsed.NextRawPart(); err != io.EOF {
		t.Fatal("missing final boundary or unexpected extra part")
	}
	if info[1].Filename != parts[1].Filename || info[3].Filename != filepath.Base(path) {
		t.Fatal("explicit or default filename changed")
	}
	info[0].Name = "changed"
	if reflect.DeepEqual(info, u.Parts()) {
		t.Fatal("preview metadata was mutable")
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(spool); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("multipart snapshot was not removed")
	}
	entries, _ := os.ReadDir(os.TempDir())
	if len(entries) != 0 {
		t.Fatal("source or final snapshot leaked")
	}
}

func TestMultipartRejectsInvalidPartsLimitsAndCancellation(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	text, nonASCII, invalid := "private-text", "日本", "\xff"
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("binary"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, parts := range [][]MultipartPart{
		nil, make([]MultipartPart, MaxMultipartParts+1),
		{{Name: "name"}}, {{Name: "name", Text: &text, File: path}},
		{{Name: "", Text: &text}}, {{Name: "a\r\nInjected: value", Text: &text}},
		{{Name: "name", Text: &invalid}}, {{Name: "name", Text: &text, Filename: "file"}},
		{{Name: "name", Text: &text, ContentType: "text/plain\r\nInjected: value"}},
		{{Name: "name", Text: &text, ContentType: "text/*"}},
		{{Name: "name", Text: &text, ContentType: "text/plain; charset=latin1"}},
		{{Name: "name", Text: &nonASCII, ContentType: "text/plain; charset=us-ascii"}},
		{{Name: "name", File: path, Filename: "../file"}},
		{{Name: "name", File: path, Filename: `..\file`}},
		{{Name: "name", File: path, Filename: "file\n"}},
		{{Name: "name", File: filepath.Dir(path)}},
		{{Name: "name", Text: &text}, {Name: "file", File: path + "-missing"}},
	} {
		if _, err := SnapshotMultipart(context.Background(), parts, 4096); err == nil {
			t.Fatal("invalid multipart input accepted")
		} else if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), text) {
			t.Fatal("multipart error exposes input")
		}
		entries, _ := os.ReadDir(os.TempDir())
		if len(entries) != 0 {
			t.Fatal("refused multipart upload leaked a snapshot")
		}
	}
	for _, limit := range []int64{-1, 0, 1, int64(len(text))} {
		if _, err := SnapshotMultipart(context.Background(), []MultipartPart{{Name: "name", Text: &text}}, limit); err == nil {
			t.Fatal("limit ignored MIME framing")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SnapshotMultipart(ctx, []MultipartPart{{Name: "name", Text: &text}}, 4096); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled multipart construction succeeded")
	}
	entries, _ := os.ReadDir(os.TempDir())
	if len(entries) != 0 {
		t.Fatal("failed upload leaked a snapshot")
	}
}
