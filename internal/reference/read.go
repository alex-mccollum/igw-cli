package reference

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	b, err := ReadFile(ctx, filepath.Join(dir, "reference.json"), MaxManifestBytes)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if jsonvalue.Validate(b) != nil || json.Unmarshal(b, &m) != nil || m.Version != Version || !namePattern.MatchString(m.Name) || m.CreatedAt.IsZero() || m.ParserVersion == "" || m.Catalog.ContractPolicy != catalog.ContractPolicy || !hashPattern.MatchString(m.Catalog.RawSHA256) || !hashPattern.MatchString(m.Catalog.DocumentSHA256) || !hashPattern.MatchString(m.Catalog.ContractSHA256) || !hashPattern.MatchString(m.ModuleInventorySHA256) || m.Qualification.Policy != QualificationPolicy || !hashPattern.MatchString(m.Qualification.TestBinarySHA256) {
		return Manifest{}, errors.New("invalid or unsupported reference manifest")
	}
	if !imagePattern.MatchString(m.Image.Reference) || !imageIDPattern.MatchString(m.Image.ConfigurationDigest) || m.Image.Platform != "linux/amd64" || m.Image.GatewayVersion == "" || !reflect.DeepEqual(m.Qualification.Scopes, QualificationScopes()) || m.Comparison.AfterIdentity != m.Catalog || m.Comparison.BeforeIdentity.ContractPolicy != catalog.ContractPolicy || m.Comparison.ContractEqual != (m.Comparison.BeforeIdentity.ContractSHA256 == m.Catalog.ContractSHA256) || m.Comparison.DocumentEqual != (m.Comparison.BeforeIdentity.DocumentSHA256 == m.Catalog.DocumentSHA256) {
		return Manifest{}, errors.New("inconsistent reference provenance or qualification scope")
	}
	compatibility := "requires_review"
	if m.Comparison.ContractEqual {
		compatibility = "unchanged_under_policy"
	}
	if m.Comparison.Compatibility != compatibility || len(m.Modules) == 0 || len(m.Modules) > 2000 {
		return Manifest{}, errors.New("invalid reference comparison or module profile")
	}
	last := ""
	for _, module := range m.Modules {
		if !strings.HasPrefix(module.ID, "com.inductiveautomation.") || module.ID <= last || len(module.ID) > 256 || module.Version == "" || len(module.Version) > 256 || module.State != "ACTIVE" || module.Collection != "healthy" {
			return Manifest{}, errors.New("invalid qualified reference module profile")
		}
		last = module.ID
	}
	expected := map[string]bool{}
	for _, path := range RequiredFiles() {
		expected[path] = true
	}
	if len(m.Files) != len(expected) {
		return Manifest{}, errors.New("reference has an incomplete payload list")
	}
	for _, file := range m.Files {
		if !expected[file.Path] || file.Bytes <= 0 || file.Bytes > fileLimit(file.Path) || !hashPattern.MatchString(file.SHA256) {
			return Manifest{}, errors.New("reference contains an invalid or duplicate payload descriptor")
		}
		delete(expected, file.Path)
		b, err := ReadFile(ctx, filepath.Join(dir, file.Path), fileLimit(file.Path))
		if err != nil {
			return Manifest{}, err
		}
		sum := sha256.Sum256(b)
		if int64(len(b)) != file.Bytes || hex.EncodeToString(sum[:]) != file.SHA256 {
			return Manifest{}, fmt.Errorf("reference payload checksum mismatch: %s", file.Path)
		}
	}
	return m, nil
}

// OpenCatalog verifies a complete reference, then parses its exact vendor JSON
// with the current parser. Historical parser receipts are never used to skip
// present validation or silently establish a target Gateway's contract.
func OpenCatalog(ctx context.Context, dir string) (Manifest, *catalog.Catalog, error) {
	m, err := Read(ctx, dir)
	if err != nil {
		return Manifest{}, nil, err
	}
	b, err := ReadFile(ctx, filepath.Join(dir, "openapi.json.gz"), fileLimit("openapi.json.gz"))
	if err != nil {
		return Manifest{}, nil, err
	}
	sum := sha256.Sum256(b)
	for _, file := range m.Files {
		if file.Path == "openapi.json.gz" && (int64(len(b)) != file.Bytes || hex.EncodeToString(sum[:]) != file.SHA256) {
			return Manifest{}, nil, errors.New("reference document changed before parsing")
		}
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
