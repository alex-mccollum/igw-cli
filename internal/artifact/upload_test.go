package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUploadSnapshotKeepsReviewedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	original := []byte("reviewed\x00binary")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	source, err := SnapshotUpload(context.Background(), path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	spool := source.file.Name()
	info, _ := os.Stat(spool)
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("upload snapshot is not private")
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(original)
	if source.Bytes() != int64(len(original)) || source.SHA256() != hex.EncodeToString(hash[:]) {
		t.Fatal("snapshot identity changed")
	}
	for n := 0; n < 2; n++ {
		reader, err := source.Open(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(reader)
		if err != nil || string(b) != string(original) {
			t.Fatal("snapshot followed changed source")
		}
		reader.Close()
		if _, err := reader.Read(make([]byte, 1)); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal("closed stream still reads")
		}
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(spool); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("snapshot was not removed")
	}
	if _, err := source.Open(context.Background()); err == nil {
		t.Fatal("closed snapshot reopened")
	}
}

func TestUploadRejectsSizeDirectoryAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int64{-1, 0, 3} {
		if _, err := SnapshotUpload(context.Background(), path, limit); err == nil {
			t.Fatal("upload size limit ignored")
		}
	}
	if _, err := SnapshotUpload(context.Background(), filepath.Dir(path), 1024); err == nil {
		t.Fatal("directory accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SnapshotUpload(ctx, path, 1024); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled snapshot succeeded")
	}
	u, err := SnapshotUpload(context.Background(), path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if _, err := u.Open(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled upload stream opened")
	}
}
