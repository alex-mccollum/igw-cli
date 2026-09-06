package reference

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var imagePattern = regexp.MustCompile(`^inductiveautomation/ignition@sha256:[a-f0-9]{64}$`)
var imageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Read verifies the manifest and every payload checksum without contacting a
// Gateway or parsing its large OpenAPI model. Checksums establish integrity,
// not publisher authenticity; callers must choose a trusted bundle source.
func Read(ctx context.Context, dir string) (Manifest, error) {
	return Directory(dir).Read(ctx)
}

// readManifest supports cheap listing. Payload integrity is checked when a
// reference is inspected, exported, or opened, never inferred from listing.
func readManifest(ctx context.Context, read fileReader) (Manifest, error) {
	b, err := read(ctx, "reference.json", MaxManifestBytes)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if jsonvalue.Validate(b) != nil || json.Unmarshal(b, &m) != nil {
		return Manifest{}, errors.New("invalid reference manifest")
	}
	if m.Version != Version {
		return Manifest{}, errors.New("unsupported reference format; export a current bundled reference or import the raw OpenAPI document")
	}
	if !namePattern.MatchString(m.Name) || m.CreatedAt.IsZero() || m.ParserVersion == "" || m.Catalog.ContractPolicy != catalog.ContractPolicy || !validIdentity(m.Catalog) || !hashPattern.MatchString(m.ModuleInventorySHA256) {
		return Manifest{}, errors.New("invalid reference identity or provenance")
	}
	if m.CapturedAt != nil && (m.CapturedAt.IsZero() || m.CapturedAt.After(m.CreatedAt)) {
		return Manifest{}, errors.New("reference capture date must precede assembly")
	}
	q := m.Qualification
	if q.Policy == "" || q.ParserVersion == "" || !validIdentity(q.Catalog) || !hashPattern.MatchString(q.TestBinarySHA256) || !hashPattern.MatchString(q.Evidence.SHA256) || q.Evidence.URI == "" || len(q.Evidence.URI) > 4096 || len(q.Scopes) == 0 {
		return Manifest{}, errors.New("invalid reference qualification summary")
	}
	if !imagePattern.MatchString(m.Image.Reference) || !imageIDPattern.MatchString(m.Image.ConfigurationDigest) || m.Image.Platform != "linux/amd64" || m.Image.GatewayVersion == "" || len(m.Modules) == 0 || len(m.Modules) > 2000 {
		return Manifest{}, errors.New("invalid reference image or module provenance")
	}
	if m.ModuleProfile != nil {
		if err := m.ModuleProfile.Validate(); err != nil {
			return Manifest{}, err
		}
	}
	last := ""
	for _, module := range m.Modules {
		if !strings.HasPrefix(module.ID, "com.inductiveautomation.") || module.ID <= last || len(module.ID) > 256 || module.Version == "" || len(module.Version) > 256 || module.Collection != "healthy" || (module.State != "ACTIVE" && module.State != "INACTIVE") {
			return Manifest{}, errors.New("invalid reference module summary")
		}
		last = module.ID
	}
	if len(m.Files) != 1 || m.Files[0].Path != "openapi.json.gz" || m.Files[0].Bytes <= 0 || m.Files[0].Bytes > fileLimit("openapi.json.gz") || !hashPattern.MatchString(m.Files[0].SHA256) {
		return Manifest{}, errors.New("reference requires one bounded OpenAPI payload")
	}
	return m, nil
}

func validIdentity(id catalog.Identity) bool {
	return hashPattern.MatchString(id.RawSHA256) && hashPattern.MatchString(id.DocumentSHA256) && hashPattern.MatchString(id.ContractSHA256) && id.ContractPolicy != ""
}

func readBundle(ctx context.Context, read fileReader) (Manifest, error) {
	m, err := readManifest(ctx, read)
	if err != nil {
		return Manifest{}, err
	}
	if _, err := readDocument(ctx, read, m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func readDocument(ctx context.Context, read fileReader, m Manifest) ([]byte, error) {
	file := m.Files[0]
	b, err := read(ctx, file.Path, fileLimit(file.Path))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	if int64(len(b)) != file.Bytes || hex.EncodeToString(sum[:]) != file.SHA256 {
		return nil, errors.New("reference OpenAPI payload checksum mismatch")
	}
	return b, nil
}

// OpenCatalog verifies a complete reference, then parses its exact vendor JSON
// with the current parser. Historical parser receipts are never used to skip
// present validation or silently establish a target Gateway's contract.
func OpenCatalog(ctx context.Context, dir string) (Manifest, *catalog.Catalog, error) {
	return Directory(dir).OpenCatalog(ctx)
}

func (bundle Bundle) OpenCatalog(ctx context.Context) (Manifest, *catalog.Catalog, error) {
	if bundle.read == nil || bundle.selector == "" {
		return Manifest{}, nil, errors.New("reference selector is required")
	}
	m, err := readManifest(ctx, bundle.read)
	if err != nil {
		return Manifest{}, nil, err
	}
	b, err := readDocument(ctx, bundle.read, m)
	if err != nil {
		return Manifest{}, nil, err
	}
	raw, err := gunzip(b)
	if err != nil {
		return Manifest{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, nil, err
	}
	c, err := catalog.Parse(raw)
	if err != nil {
		return Manifest{}, nil, err
	}
	if c.Identity() != m.Catalog {
		c.Close()
		return Manifest{}, nil, errors.New("reference catalog identity does not match its manifest")
	}
	if err := ctx.Err(); err != nil {
		c.Close()
		return Manifest{}, nil, err
	}
	return m, c, nil
}

func fileLimit(path string) int64 {
	if path == "openapi.json.gz" {
		return catalog.MaxDocumentBytes + (1 << 20)
	}
	return 4 << 20
}

func gunzip(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("invalid compressed reference document")
	}
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, catalog.MaxDocumentBytes+1))
	if err != nil || len(raw) > catalog.MaxDocumentBytes {
		return nil, errors.New("invalid or oversized compressed reference document")
	}
	return raw, nil
}

// ReadFile bounds contributor-controlled artifacts and rejects special files.
func ReadFile(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("reference requires bounded regular files")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("reference input changed before reading")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("reference input exceeds size limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b, nil
}
