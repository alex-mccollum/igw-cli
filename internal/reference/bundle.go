package reference

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

// Only reviewed bundle payloads are included; the contributor README is not.
//
//go:embed bundles/*/reference.json bundles/*/openapi.json.gz
var bundled embed.FS

type fileReader func(context.Context, string, int64) ([]byte, error)

// Bundle reads a fixed set of local or embedded files through the same checks.
// It has no target, credentials, cache, or network client.
type Bundle struct {
	selector string
	origin   string
	read     fileReader
}

// Summary identifies reference evidence without claiming live target freshness.
// ParserVersion identifies qualification; InspectionParserVersion is set only
// when a command has reparsed the document in this invocation. InspectionCatalog
// reports current policy identity separately from the immutable recorded one.
type Summary struct {
	Selector                string                   `json:"selector"`
	SourceKind              string                   `json:"sourceKind"`
	Origin                  string                   `json:"origin"`
	Version                 string                   `json:"version"`
	Name                    string                   `json:"name"`
	CreatedAt               time.Time                `json:"createdAt"`
	Image                   Image                    `json:"image"`
	ModuleInventorySHA256   string                   `json:"moduleInventorySha256"`
	ModuleProfile           *moduleprofile.Selection `json:"moduleProfile,omitempty"`
	ModuleCount             int                      `json:"moduleCount"`
	ActiveModuleCount       int                      `json:"activeModuleCount"`
	Catalog                 catalog.Identity         `json:"catalog"`
	ParserVersion           string                   `json:"parserVersion"`
	InspectionParserVersion string                   `json:"inspectionParserVersion,omitempty"`
	InspectionCatalog       *catalog.Identity        `json:"inspectionCatalog,omitempty"`
	Qualification           Qualification            `json:"qualification"`
}

func (b Bundle) Summary(m Manifest) Summary {
	active := 0
	for _, module := range m.Modules {
		if module.State == "ACTIVE" {
			active++
		}
	}
	return Summary{Selector: b.selector, SourceKind: "reference", Origin: b.origin,
		Version: m.Version, Name: m.Name, CreatedAt: m.CreatedAt, Image: m.Image,
		ModuleInventorySHA256: m.ModuleInventorySHA256, ModuleProfile: m.ModuleProfile, ModuleCount: len(m.Modules), ActiveModuleCount: active,
		Catalog: m.Catalog, ParserVersion: m.Qualification.ParserVersion, Qualification: m.Qualification}
}

func Directory(dir string) Bundle {
	return Bundle{selector: dir, origin: "directory", read: func(ctx context.Context, name string, limit int64) ([]byte, error) {
		return ReadFile(ctx, filepath.Join(dir, name), limit)
	}}
}

// Select uses an exact bundled selector when present, otherwise a directory.
// Prefix a relative directory with ./ to disambiguate a bundled name.
func Select(selector string) Bundle {
	if namePattern.MatchString(selector) {
		if info, err := fs.Stat(bundled, "bundles/"+selector); err == nil && info.IsDir() {
			return Bundle{selector: selector, origin: "bundled", read: func(ctx context.Context, name string, limit int64) ([]byte, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				data, err := bundled.ReadFile("bundles/" + selector + "/" + name)
				if int64(len(data)) > limit {
					return nil, errors.New("embedded reference exceeds size limit")
				}
				return data, err
			}}
		}
	}
	return Directory(selector)
}

func (b Bundle) Read(ctx context.Context) (Manifest, error) {
	if b.read == nil || b.selector == "" {
		return Manifest{}, errors.New("reference selector is required")
	}
	return readBundle(ctx, b.read)
}

// List reads sorted embedded manifests without decompressing or reading payloads.
func List(ctx context.Context) ([]Summary, error) {
	entries, err := bundled.ReadDir("bundles")
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundle := Select(entry.Name())
		m, err := readManifest(ctx, bundle.read)
		if err != nil {
			return nil, err
		}
		items = append(items, bundle.Summary(m))
	}
	return items, ctx.Err()
}

// Export preserves exact payload and manifest bytes. Verification precedes
// publication; a new directory is required and the manifest is published last.
// On failure any partial directory remains identifiable and is never replaced.
func (b Bundle) Export(ctx context.Context, dir string, expected catalog.Identity) (Manifest, error) {
	if dir == "" || b.read == nil || b.selector == "" {
		return Manifest{}, errors.New("reference selector and new output directory are required")
	}
	payloads := make(map[string][]byte)
	m, err := readBundle(ctx, func(ctx context.Context, name string, limit int64) ([]byte, error) {
		data, err := b.read(ctx, name, limit)
		if err == nil {
			payloads[name] = data
		}
		return data, err
	})
	if err != nil {
		return Manifest{}, err
	}
	if m.Catalog != expected {
		return Manifest{}, errors.New("reference identity changed before export")
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return Manifest{}, err
	}
	for _, name := range append(RequiredFiles(), "reference.json") {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		if err := exportFile(filepath.Join(dir, name), payloads[name]); err != nil {
			return Manifest{}, err
		}
	}
	if _, err := Read(ctx, dir); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func exportFile(path string, data []byte) error {
	w, err := artifact.New(path, false)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err == nil {
		_, err = w.Commit()
	}
	return errors.Join(err, w.Abort())
}
