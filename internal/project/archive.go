// Package project implements project archive transfer and verification.
package project

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
)

type File struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Encoding string `json:"encoding"`
}
type Manifest struct {
	SHA256 string `json:"sha256"`
	Files  []File `json:"files"`
}

var ErrArchive = errors.New("invalid or unsupported project archive; require a standard ZIP with unique relative paths and project.json")

// Inspect hashes file content independently of ZIP timestamps/order. JSON
// entries up to 4 MiB use exact canonical JSON; larger entries use their bytes.
// Neither archive extraction nor execution is involved.
func Inspect(ctx context.Context, source *artifact.Upload) (manifestResult Manifest, returnedErr error) {
	defer func() {
		if ctx.Err() != nil {
			manifestResult, returnedErr = Manifest{}, ctx.Err()
		}
	}()
	if source == nil {
		return Manifest{}, ErrArchive
	}
	reader, err := source.Open(ctx)
	if err != nil {
		return Manifest{}, err
	}
	defer reader.Close()
	if err := checkDirectory(reader, source.Bytes()); err != nil {
		return Manifest{}, err
	}
	archive, err := zip.NewReader(reader, source.Bytes())
	if err != nil {
		return Manifest{}, ErrArchive
	}
	if len(archive.File) > 10000 {
		return Manifest{}, ErrArchive
	}
	manifest := Manifest{Files: []File{}}
	seen := map[string]bool{}
	var expanded uint64
	projectFile := false
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if name == "" || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00:") || seen[name] {
			return Manifest{}, ErrArchive
		}
		seen[name] = true
		if entry.FileInfo().IsDir() {
			continue
		}
		if name == "project.json" {
			projectFile = true
			if entry.UncompressedSize64 > 4<<20 {
				return Manifest{}, ErrArchive
			}
		}
		if !entry.Mode().IsRegular() || entry.UncompressedSize64 > uint64(artifact.DefaultUploadLimit) {
			return Manifest{}, ErrArchive
		}
		expanded += entry.UncompressedSize64
		if expanded > uint64(artifact.DefaultUploadLimit) {
			return Manifest{}, ErrArchive
		}
		file, err := entry.Open()
		if err != nil {
			return Manifest{}, ErrArchive
		}
		bounded := io.LimitReader(contextReader{ctx, file}, int64(entry.UncompressedSize64)+1)
		var digest string
		encoding := "bytes"
		if strings.HasSuffix(name, ".json") && entry.UncompressedSize64 <= 4<<20 {
			raw, readErr := io.ReadAll(bounded)
			closeErr := file.Close()
			if readErr != nil || closeErr != nil || uint64(len(raw)) != entry.UncompressedSize64 {
				return Manifest{}, ErrArchive
			}
			raw, err = jsonvalue.Canonical(raw)
			if err != nil {
				return Manifest{}, ErrArchive
			}
			if name == "project.json" && (len(raw) == 0 || raw[0] != '{') {
				return Manifest{}, ErrArchive
			}
			hash := sha256.Sum256(raw)
			digest = hex.EncodeToString(hash[:])
			encoding = "canonical_json"
		} else {
			hash := sha256.New()
			n, readErr := io.Copy(hash, bounded)
			closeErr := file.Close()
			if readErr != nil || closeErr != nil || uint64(n) != entry.UncompressedSize64 {
				return Manifest{}, ErrArchive
			}
			digest = hex.EncodeToString(hash.Sum(nil))
		}
		manifest.Files = append(manifest.Files, File{name, digest, encoding})
	}
	if !projectFile || len(manifest.Files) == 0 {
		return Manifest{}, ErrArchive
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	raw, _ := json.Marshal(manifest.Files)
	hash := sha256.Sum256(raw)
	manifest.SHA256 = hex.EncodeToString(hash[:])
	return manifest, nil
}

// Bound central-directory allocation before archive/zip parses entries. ZIP64,
// multi-disk and prefixed/self-extracting archives are outside this contract.
func checkDirectory(reader io.ReaderAt, size int64) error {
	if size < 22 {
		return ErrArchive
	}
	var magic [4]byte
	if _, err := reader.ReadAt(magic[:], 0); err != nil || binary.LittleEndian.Uint32(magic[:]) != 0x04034b50 {
		return ErrArchive
	}
	count := int64(65557)
	if size < count {
		count = size
	}
	tail := make([]byte, count)
	if _, err := reader.ReadAt(tail, size-count); err != nil {
		return ErrArchive
	}
	for i := len(tail) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) != 0x06054b50 || i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) != len(tail) {
			continue
		}
		end := tail[i:]
		records := binary.LittleEndian.Uint16(end[10:])
		directorySize := int64(binary.LittleEndian.Uint32(end[12:]))
		directoryOffset := int64(binary.LittleEndian.Uint32(end[16:]))
		if binary.LittleEndian.Uint16(end[4:]) != 0 || binary.LittleEndian.Uint16(end[6:]) != 0 || records == 0 || records > 10000 || binary.LittleEndian.Uint16(end[8:]) != records || directorySize > 8<<20 || directoryOffset+directorySize != size-count+int64(i) {
			return ErrArchive
		}
		return nil
	}
	return ErrArchive
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
