package catalog

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
)

type Metadata struct {
	Version        int            `json:"version"`
	Target         Target         `json:"target"`
	Source         string         `json:"source"`
	SourceKind     string         `json:"sourceKind"`
	FetchedAt      time.Time      `json:"fetchedAt"`
	VerifiedAt     time.Time      `json:"verifiedAt"`
	RawSHA256      string         `json:"rawSha256"`
	ContractSHA256 string         `json:"contractSha256"`
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

// Store uses immutable content-addressed blobs and immutable validation
// receipts. Atomic no-clobber publication coordinates writers without lock
// files or a mutable latest pointer that can roll back after a concurrent sync.
type Store struct{ Dir string }

func (s Store) Save(snapshot *Snapshot) error {
	if s.Dir == "" {
		return errors.New("catalog store directory is required")
	}
	m := &snapshot.Metadata
	if m.Version != 1 || m.VerifiedAt.IsZero() || snapshot.Catalog == nil {
		return errors.New("invalid snapshot metadata")
	}
	if m.RawSHA256 != snapshot.Catalog.RawHash() || m.ContractSHA256 != snapshot.Catalog.ContractHash() {
		return errors.New("snapshot hashes do not match document")
	}
	m.Compatibility = snapshot.Catalog.Compatibility()
	if target, err := NewTarget(m.Target.Profile, m.Target.URL); err != nil || target != m.Target {
		return errors.New("snapshot target must be normalized")
	}
	blobs := filepath.Join(s.Dir, "blobs")
	receipts := filepath.Join(s.Dir, "targets", m.Target.Key())
	for _, dir := range []string{blobs, receipts} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	raw := snapshot.Catalog.Raw()
	blobPath := filepath.Join(blobs, m.RawSHA256+".json")
	if err := publish(blobPath, raw); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := readBounded(blobPath, MaxDocumentBytes)
		if readErr != nil || !bytes.Equal(existing, raw) {
			return errors.New("stored catalog blob is corrupt")
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%s.json", m.VerifiedAt.UnixNano(), hex.EncodeToString(nonce[:]))
	return publish(filepath.Join(receipts, name), b)
}

func publish(path string, b []byte) error {
	w, err := artifact.New(path, false)
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

func (s Store) Load(target Target) (*Snapshot, error) {
	dir := filepath.Join(s.Dir, "targets", target.Key())
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	corrupt := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		snapshot, err := s.loadReceipt(filepath.Join(dir, entry.Name()), target)
		if err != nil {
			corrupt = true
			continue
		}
		if corrupt {
			snapshot.Warnings = append(snapshot.Warnings, "Ignored an invalid newer cached snapshot; using the last valid snapshot.")
		}
		return snapshot, nil
	}
	if corrupt {
		return nil, errors.New("no valid snapshot remains in the target cache")
	}
	return nil, os.ErrNotExist
}

func (s Store) loadReceipt(path string, target Target) (*Snapshot, error) {
	b, err := readBounded(path, 1<<20)
	if err != nil {
		return nil, err
	}
	var m Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Version != 1 || m.Target != target || m.VerifiedAt.IsZero() || !validDigest(m.RawSHA256) {
		return nil, errors.New("invalid snapshot receipt")
	}
	if m.SourceKind == "gateway" {
		expected, err := target.Endpoint("/openapi.json")
		if err != nil || m.Source != expected {
			return nil, errors.New("snapshot source does not match target")
		}
	}
	raw, err := readBounded(filepath.Join(s.Dir, "blobs", m.RawSHA256+".json"), MaxDocumentBytes)
	if err != nil {
		return nil, err
	}
	if digest(raw) != m.RawSHA256 {
		return nil, errors.New("catalog blob checksum mismatch")
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	if c.ContractHash() != m.ContractSHA256 {
		c.Close()
		return nil, errors.New("catalog contract checksum mismatch")
	}
	m.Compatibility = c.Compatibility()
	return &Snapshot{Metadata: m, Catalog: c}, nil
}

func validDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && strings.ToLower(value) == value
}

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
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
