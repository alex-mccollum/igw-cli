package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

const DefaultUploadLimit int64 = 1 << 30

// Upload is a private immutable snapshot of a regular input file. Its bytes
// and digest describe the same content used for validation and transmission,
// even if the original pathname changes afterwards. Close removes the spool.
type Upload struct {
	mu          sync.Mutex
	file        *os.File
	bytes       int64
	sha256      string
	contentType string
	parts       []MultipartPartInfo
}

func SnapshotUpload(ctx context.Context, path string, limit int64) (*Upload, error) {
	if limit <= 0 {
		return nil, errors.New("upload limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("upload input must be a regular file")
	}
	input, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not open upload file")
	}
	defer input.Close()
	before, err := input.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, errors.New("upload input must be a regular file")
	}
	if before.Size() > limit {
		return nil, errors.New("upload exceeds size limit")
	}
	spool, err := os.CreateTemp("", "igw-upload-*")
	if err != nil {
		return nil, errors.New("could not create private upload snapshot")
	}
	u := &Upload{file: spool}
	keep := false
	defer func() {
		if !keep {
			_ = u.Close()
		}
	}()
	hash := sha256.New()
	reader := &uploadReader{ctx: ctx, reader: io.NewSectionReader(input, 0, before.Size())}
	u.bytes, err = io.CopyBuffer(io.MultiWriter(spool, hash), reader, make([]byte, 64<<10))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("could not snapshot upload file")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	after, err := input.Stat()
	if err != nil || u.bytes != before.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("upload file changed while creating its snapshot")
	}
	u.sha256 = hex.EncodeToString(hash.Sum(nil))
	keep = true
	return u, nil
}

func (u *Upload) Bytes() int64               { return u.bytes }
func (u *Upload) SHA256() string             { return u.sha256 }
func (u *Upload) ContentType() string        { return u.contentType }
func (u *Upload) Parts() []MultipartPartInfo { return append([]MultipartPartInfo(nil), u.parts...) }

type UploadReader interface {
	io.ReadCloser
	io.ReaderAt
}

func (u *Upload) Open(ctx context.Context) (UploadReader, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.file == nil {
		return nil, errors.New("upload snapshot is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &uploadReader{ctx: ctx, reader: io.NewSectionReader(u.file, 0, u.bytes)}, nil
}

func (u *Upload) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.file == nil {
		return nil
	}
	name := u.file.Name()
	err := u.file.Close()
	u.file = nil
	return errors.Join(err, os.Remove(name))
}

type uploadReader struct {
	ctx    context.Context
	reader *io.SectionReader
	closed atomic.Bool
}

func (r *uploadReader) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func (r *uploadReader) Close() error { r.closed.Store(true); return nil }

func (r *uploadReader) ReadAt(p []byte, off int64) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.ReadAt(p, off)
}
