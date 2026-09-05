// Package artifact publishes complete downloads without exposing partial files.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
)

type Info struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Writer owns one private temporary file in the destination directory. Callers
// must defer Abort, then Commit only after the complete response is verified.
// A Writer is used by one transfer at a time.
type Writer struct {
	file      *os.File
	temp      string
	path      string
	overwrite bool
	hash      hash.Hash
	bytes     int64
	err       error
}

func New(path string, overwrite bool) (*Writer, error) {
	if path == "" {
		return nil, errors.New("artifact destination is required")
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !overwrite {
			return nil, fmt.Errorf("destination exists; use --overwrite: %w", os.ErrExist)
		}
		if info.IsDir() {
			return nil, errors.New("artifact destination is a directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".igw-artifact-*")
	if err != nil {
		return nil, err
	}
	return &Writer{file: f, temp: f.Name(), path: path, overwrite: overwrite, hash: sha256.New()}, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.file.Write(p)
	_, _ = w.hash.Write(p[:n])
	w.bytes += int64(n)
	w.err = err
	return n, err
}

func (w *Writer) Commit() (Info, error) {
	if w.file == nil {
		return Info{}, os.ErrClosed
	}
	if w.err != nil {
		return Info{}, w.err
	}
	if err := w.file.Sync(); err != nil {
		return Info{}, err
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return Info{}, err
	}
	if w.overwrite {
		err = os.Rename(w.temp, w.path)
	} else {
		// Link is atomic and refuses to replace a destination created since New.
		// Both paths are on the same filesystem. Unsupported filesystems fail
		// explicitly instead of falling back to a racy check followed by rename.
		err = os.Link(w.temp, w.path)
	}
	if err != nil {
		return Info{}, err
	}
	if w.overwrite {
		w.temp = ""
	}
	return Info{Path: w.path, Bytes: w.bytes, SHA256: hex.EncodeToString(w.hash.Sum(nil))}, nil
}

// Abort is also safe after Commit: only the owned temporary name is removed.
func (w *Writer) Abort() error {
	var closeErr, removeErr error
	if w.file != nil {
		closeErr = w.file.Close()
		w.file = nil
	}
	if w.temp != "" {
		removeErr = os.Remove(w.temp)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		if removeErr == nil {
			w.temp = ""
		}
	}
	return errors.Join(closeErr, removeErr)
}
