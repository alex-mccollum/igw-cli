package project

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
)

func archiveFixture(t *testing.T, entries []struct{ name, body string }, stamp time.Time) *artifact.Upload {
	t.Helper()
	path := filepath.Join(t.TempDir(), "project.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		h.SetModTime(stamp)
		file, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	u, err := artifact.SnapshotUpload(context.Background(), path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { u.Close() })
	return u
}

func TestArchiveIdentityIgnoresContainerOrderAndJSONFormatting(t *testing.T) {
	first := archiveFixture(t, []struct{ name, body string }{{"project.json", `{"title":"test","enabled":false}`}, {"view.json", `{"n":9007199254740993,"size":1.00}`}, {"data.bin", "opaque\x00bytes"}}, time.Unix(0, 0))
	second := archiveFixture(t, []struct{ name, body string }{{"data.bin", "opaque\x00bytes"}, {"view.json", `{"size":1e0, "n":9007199254740993}`}, {"project.json", `{"enabled":false,"title":"test"}`}}, time.Unix(1700000000, 0))
	a, err := Inspect(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Inspect(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != b.SHA256 || len(a.Files) != 3 || first.SHA256() == second.SHA256() {
		t.Fatal("archive identity follows container metadata")
	}
	changed := archiveFixture(t, []struct{ name, body string }{{"project.json", `{"title":"test","enabled":false}`}, {"view.json", `{"n":9007199254740992,"size":1}`}, {"data.bin", "opaque\x00bytes"}}, time.Unix(0, 0))
	c, err := Inspect(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 == c.SHA256 {
		t.Fatal("archive digest lost numeric precision")
	}
}

func TestArchiveRejectsAmbiguityTraversalAndMissingManifest(t *testing.T) {
	for _, entries := range [][]struct{ name, body string }{
		{{"project.json", `{}`}, {"project.json", `{}`}},
		{{"project.json", `{}`}, {"../escape", "x"}},
		{{"project.json", `{}`}, {"C:/escape", "x"}},
		{{"project.json", `{}`}, {"a\\b", "x"}},
		{{"other.json", `{}`}},
		{{"project.json", `null`}},
		{{"project.json", `{"enabled":true,"enabled":false}`}},
	} {
		u := archiveFixture(t, entries, time.Unix(0, 0))
		if _, err := Inspect(context.Background(), u); err == nil {
			t.Fatal("unsafe project archive accepted")
		}
	}
}
