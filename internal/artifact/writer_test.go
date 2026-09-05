package artifact

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishCompleteFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "backup.gwbk")
	w, err := New(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	if _, err := w.Write([]byte("complete")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published before commit")
	}
	info, err := w.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != path || info.Bytes != 8 || info.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte("complete"))) {
		t.Fatalf("wrong metadata: %+v", info)
	}
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "complete" {
		t.Fatalf("published data: %q, %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain: %v, %v", entries, err)
	}
}

func TestAbortPreservesExistingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "backup")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path, false); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected no-clobber error: %v", err)
	}
	w, err := New(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	if _, err := w.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatalf("lost original: %q, %v", data, err)
	}
}

func TestConcurrentPublicationDoesNotClobber(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "backup")
	w, err := New(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	if _, err := w.Write([]byte("download")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other writer"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected competing file to win: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "other writer" {
		t.Fatalf("clobbered competing file: %q, %v", data, err)
	}
}

func TestExplicitOverwriteReplacesCompleteFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "backup")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err := New(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("replacement failed: %q, %v", data, err)
	}
}
