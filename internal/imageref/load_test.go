package imageref

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadVerifiesSavedImageChainOffline(t *testing.T) {
	index, body := fixture(t, ociIndex, ociManifest)
	candidate, err := resolve(context.Background(), "8.3", registry(t, index, body, nil), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "saved")
	if err := candidate.Save(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := loaded.ConfigurationDigest()
	if err != nil || digest != "sha256:"+strings.Repeat("a", 64) || !bytes.Equal(loaded.Index, index) || !bytes.Equal(loaded.Manifest, body) {
		t.Fatalf("image configuration link lost: %s %v", digest, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), dir); err == nil {
		t.Fatal("corrupt saved manifest accepted")
	}
}

func TestCandidateRejectsForgedProvenance(t *testing.T) {
	index, body := fixture(t, ociIndex, ociManifest)
	good, err := resolve(context.Background(), "8.3", registry(t, index, body, nil), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		alter func(*Candidate)
	}{
		{"version", func(c *Candidate) { c.Resolution.Version = 2 }},
		{"repository", func(c *Candidate) { c.Resolution.Repository = "foreign/image" }},
		{"tag", func(c *Candidate) { c.Resolution.Tag = "latest" }},
		{"time", func(c *Candidate) { c.Resolution.ResolvedAt = time.Time{} }},
		{"image", func(c *Candidate) { c.Resolution.Image = Repository + ":8.3" }},
		{"platform", func(c *Candidate) { c.Resolution.Platform = "linux/arm64" }},
		{"index", func(c *Candidate) { c.Index = append([]byte{}, c.Index[:len(c.Index)-1]...) }},
		{"manifest", func(c *Candidate) { c.Manifest = []byte("{}") }},
		{"manifest link", func(c *Candidate) { c.Resolution.ManifestDigest = "sha256:" + strings.Repeat("b", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := good
			tc.alter(&changed)
			if _, err := changed.ConfigurationDigest(); err == nil {
				t.Fatal("forged resolution accepted")
			}
			if err := changed.Save(context.Background(), filepath.Join(t.TempDir(), "candidate")); err == nil {
				t.Fatal("forged resolution published")
			}
		})
	}
}

func TestLoadRejectsMissingReceiptAndNonRegularFiles(t *testing.T) {
	index, body := fixture(t, ociIndex, ociManifest)
	candidate, err := resolve(context.Background(), "8.3", registry(t, index, body, nil), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "saved")
	if err := candidate.Save(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "resolution.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), dir); err == nil {
		t.Fatal("incomplete candidate accepted")
	}
	if err := os.Remove(filepath.Join(dir, "index.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "index.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), dir); err == nil {
		t.Fatal("non-regular input accepted")
	}
}
