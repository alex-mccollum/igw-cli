package cli

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

func TestProjectInspectDoesNotLoadConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create("project.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`{"title":"Local"}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	app, out, _ := testApp(t, nil)
	app.ReadConfig = func() (config.File, error) { t.Fatal("local inspection loaded config"); return config.File{}, nil }
	if err := app.Run(context.Background(), []string{"project", "inspect", path, "--json"}); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	data := got.Data.(map[string]any)
	if !got.OK || len(data["sha256"].(string)) != 64 || len(data["files"].([]any)) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestTransferUsageRejectsBeforeReadingConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"project", "import", "copy", "--in", "input.zip"},
		{"project", "import", "copy", "--in", "input.zip", "--overwrite", "--yes"},
		{"project", "import", "copy", "--in", "input.zip", "--dry-run", "--offline"},
		{"project", "list", "--limit", "0"},
		{"project", "get", "../other"},
		{"project", "export", "copy"},
		{"tag", "import", "--in", "input.json"},
		{"tag", "import", "--in", "input.unknown", "--yes"},
		{"tag", "import", "--in", "input.json", "--yes", "--collision-policy", "guess"},
		{"tag", "import", "--in", "input.json", "--dry-run", "--offline"},
		{"tag", "export", "--out", "tags.json", "--path", "[other]tag"},
	} {
		app, out, _ := testApp(t, nil)
		app.ReadConfig = func() (config.File, error) {
			t.Fatal("invalid transfer loaded configuration")
			return config.File{}, nil
		}
		err := app.Run(context.Background(), append(args, "--json"))
		got := decodeResult(t, out)
		if err == nil || got.OK || got.Error.Code != 2 {
			t.Fatalf("invalid transfer accepted: %v", args)
		}
	}
}
