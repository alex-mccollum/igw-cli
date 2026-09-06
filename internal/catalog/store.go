package catalog

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/fslock"
)

const SnapshotVersion = 3

type Metadata struct {
	Version        int            `json:"version"`
	Target         Target         `json:"target"`
	Source         string         `json:"source"`
	SourceKind     string         `json:"sourceKind"`
	FetchedAt      time.Time      `json:"fetchedAt"`
	VerifiedAt     time.Time      `json:"verifiedAt"`
	RawSHA256      string         `json:"rawSha256"`
	DocumentSHA256 string         `json:"documentSha256"`
	ContractSHA256 string         `json:"contractSha256"`
	ContractPolicy string         `json:"contractPolicy"`
	ParserVersion  string         `json:"parserVersion"`
	ETag           string         `json:"etag,omitempty"`
	LastModified   string         `json:"lastModified,omitempty"`
	GatewayVersion string         `json:"gatewayVersion,omitempty"`
	Modules        []string       `json:"modules,omitempty"`
	Compatibility  *Compatibility `json:"compatibility,omitempty"`
}

type Snapshot struct {
	Metadata Metadata `json:"metadata"`
	Stale    bool     `json:"stale"`
	Warnings []string `json:"warnings,omitempty"`
	Catalog  *Catalog `json:"-"`
}

func (s *Snapshot) Close() {
	if s != nil && s.Catalog != nil {
		s.Catalog.Close()
	}
}

// Store keeps at most two snapshots per target. Locks protect publication and
// copying bytes for readers; HTTP and schema compilation never hold the lock.
type Store struct{ Dir string }

func (s Store) targetDir(target Target) string {
	return filepath.Join(s.Dir, "targets", target.Key())
}

func (s Store) Save(ctx context.Context, snapshot *Snapshot) error {
	if s.Dir == "" {
		return errors.New("catalog store directory is required")
	}
	if snapshot == nil || snapshot.Catalog == nil {
		return errors.New("invalid snapshot metadata")
	}
	m := &snapshot.Metadata
	if m.Version != SnapshotVersion || m.VerifiedAt.IsZero() {
		return errors.New("invalid snapshot metadata")
	}
	if m.RawSHA256 != snapshot.Catalog.RawHash() || m.ContractSHA256 != snapshot.Catalog.ContractHash() {
		return errors.New("snapshot hashes do not match document")
	}
	if target, err := NewTarget(m.Target.Profile, m.Target.URL); err != nil || target != m.Target {
		return errors.New("snapshot target must be normalized")
	}
	m.Compatibility = snapshot.Catalog.Compatibility()
	m.DocumentSHA256, m.ContractPolicy, m.ParserVersion = snapshot.Catalog.DocumentHash(), ContractPolicy, ParserVersion
	dir := s.targetDir(m.Target)
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0700); err != nil {
		return err
	}
	lock, err := fslock.Acquire(ctx, filepath.Join(dir, ".lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	current, currentErr := readStored(dir, "current.json", m.Target)
	previous, _ := readStored(dir, "previous.json", m.Target)
	blobVerified := false
	for _, old := range []*storedSnapshot{current, previous} {
		if old != nil && old.metadata.VerifiedAt.After(m.VerifiedAt) {
			return nil
		}
		if old != nil && old.metadata.RawSHA256 == m.RawSHA256 {
			blobVerified = true
		}
	}
	// readStored verifies the bytes, not just the metadata. Revalidation can
	// reuse that blob; missing or corrupt bytes still need atomic replacement.
	if !blobVerified {
		if err := publish(filepath.Join(dir, "blobs", m.RawSHA256+".json"), snapshot.Catalog.Raw()); err != nil {
			return err
		}
	}
	if currentErr == nil {
		if err := publishMetadata(filepath.Join(dir, "previous.json"), current.metadata); err != nil {
			return err
		}
		previous = current
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishMetadata(filepath.Join(dir, "current.json"), *m); err != nil {
		return err
	}
	// A crash before publication leaves the old current intact. A crash after
	// publication can leave orphan blobs; the next successful save collects them.
	keep := map[string]bool{m.RawSHA256 + ".json": true}
	if previous != nil {
		keep[previous.metadata.RawSHA256+".json"] = true
	}
	for _, directory := range []string{dir, filepath.Join(dir, "blobs")} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			abandoned := strings.HasPrefix(name, ".igw-artifact-")
			obsolete := directory != dir && strings.HasSuffix(name, ".json") && validDigest(strings.TrimSuffix(name, ".json")) && !keep[name]
			if entry.Type().IsRegular() && (abandoned || obsolete) {
				if err := os.Remove(filepath.Join(directory, name)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Metadata checksums detect accidental damage before rotation can replace a
// valid fallback. They provide integrity, not authenticity of a local cache.
type storedMetadata struct {
	Metadata Metadata `json:"metadata"`
	SHA256   string   `json:"sha256"`
}

func publishMetadata(path string, m Metadata) error {
	checksum, err := canonicalDigest(m, "")
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(storedMetadata{Metadata: m, SHA256: checksum}, "", "  ")
	if err != nil {
		return err
	}
	return publish(path, b)
}

func publish(path string, b []byte) error {
	w, err := artifact.New(path, true)
	if err != nil {
		return err
	}
	defer w.Abort()
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = w.Commit()
	return err
}

type storedSnapshot struct {
	metadata Metadata
	raw      []byte
}

func (s Store) Load(ctx context.Context, target Target) (*Snapshot, error) {
	dir := s.targetDir(target)
	lock, err := fslock.Acquire(ctx, filepath.Join(dir, ".lock"))
	if err != nil {
		return nil, err
	}
	// Copy both candidates before releasing the lock so cleanup cannot race a
	// reader. Only one is normally parsed, and each document has a fixed size cap.
	current, currentErr := readStored(dir, "current.json", target)
	previous, previousErr := readStored(dir, "previous.json", target)
	lock.Close()
	for i, stored := range []*storedSnapshot{current, previous} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if stored == nil {
			continue
		}
		c, err := Parse(stored.raw)
		if err != nil {
			continue
		}
		m := stored.metadata
		if c.ContractHash() != m.ContractSHA256 || c.DocumentHash() != m.DocumentSHA256 {
			c.Close()
			continue
		}
		m.Compatibility, m.ParserVersion = c.Compatibility(), ParserVersion
		snapshot := &Snapshot{Metadata: m, Catalog: c}
		if i > 0 {
			snapshot.Warnings = []string{"Ignored an invalid current cached snapshot; using the previous snapshot."}
		}
		return snapshot, nil
	}
	if errors.Is(currentErr, os.ErrNotExist) && errors.Is(previousErr, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	return nil, errors.New("no valid snapshot remains in the target cache; run spec sync or import a snapshot")
}

func readStored(dir, name string, target Target) (*storedSnapshot, error) {
	b, err := readBounded(filepath.Join(dir, name), 1<<20)
	if err != nil {
		return nil, err
	}
	var record storedMetadata
	if err := json.Unmarshal(b, &record); err != nil {
		return nil, err
	}
	m := record.Metadata
	checksum, err := canonicalDigest(m, "")
	if err != nil || checksum != record.SHA256 {
		return nil, errors.New("catalog metadata checksum mismatch")
	}
	if m.Version != SnapshotVersion || m.ContractPolicy != ContractPolicy || m.Target != target || m.VerifiedAt.IsZero() || !validDigest(m.RawSHA256) || !validDigest(m.DocumentSHA256) || !validDigest(m.ContractSHA256) {
		return nil, errors.New("invalid snapshot metadata; run spec sync to rebuild the cache")
	}
	if m.SourceKind == "gateway" {
		expected, err := target.Endpoint("/openapi.json")
		if err != nil || m.Source != expected {
			return nil, errors.New("snapshot source does not match target")
		}
	}
	raw, err := readBounded(filepath.Join(dir, "blobs", m.RawSHA256+".json"), MaxDocumentBytes)
	if err != nil {
		return nil, err
	}
	if digest(raw) != m.RawSHA256 {
		return nil, errors.New("catalog blob checksum mismatch")
	}
	return &storedSnapshot{metadata: m, raw: raw}, nil
}

func validDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && strings.ToLower(value) == value
}

func readBounded(path string, limit int64) ([]byte, error) {
	prior, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !prior.Mode().IsRegular() {
		return nil, errors.New("catalog file must be regular")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(prior, info) || info.Size() > limit {
		return nil, errors.New("catalog file exceeds limit or is not a regular file")
	}
	// Limit the read too: another local process could change the file after Stat.
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("catalog file exceeds size limit")
	}
	return b, nil
}
