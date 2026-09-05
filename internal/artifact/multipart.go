package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxMultipartParts = 256

// MultipartPart selects exact UTF-8 text or a regular local file. File paths
// stay local; only the basename (or explicit Filename) appears on the wire.
type MultipartPart struct {
	Name        string  `json:"name"`
	Text        *string `json:"text,omitempty"`
	File        string  `json:"file,omitempty"`
	Filename    string  `json:"filename,omitempty"`
	ContentType string  `json:"contentType,omitempty"`
}

type MultipartPartInfo struct {
	Name        string `json:"name"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"contentType"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
}

// SnapshotMultipart encodes ordered parts into one private, bounded snapshot.
// MIME framing counts toward limit. At most one source-file snapshot is open
// at a time; nothing reads a whole file into memory. The returned Upload owns
// the final spool and must be closed by its caller.
func SnapshotMultipart(ctx context.Context, parts []MultipartPart, limit int64) (*Upload, error) {
	if limit <= 0 {
		return nil, errors.New("upload limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(parts) == 0 || len(parts) > MaxMultipartParts {
		return nil, errors.New("multipart input requires between 1 and 256 parts")
	}
	metadata := make([]MultipartPartInfo, len(parts))
	for n, part := range parts {
		info, err := multipartMetadata(part)
		if err != nil {
			return nil, err
		}
		metadata[n] = info
	}
	spool, err := os.CreateTemp("", "igw-upload-*")
	if err != nil {
		return nil, errors.New("could not create private multipart snapshot")
	}
	u := &Upload{file: spool, parts: metadata}
	keep := false
	defer func() {
		if !keep {
			_ = u.Close()
		}
	}()
	hash := sha256.New()
	bounded := &multipartSink{ctx: ctx, writer: io.MultiWriter(spool, hash), limit: limit}
	writer := multipart.NewWriter(bounded)
	u.contentType = writer.FormDataContentType()
	for n, part := range parts {
		info := &u.parts[n]
		headers := make(textproto.MIMEHeader)
		if part.Text != nil {
			// Use the standard library's quoting convention without adding a
			// filename to text fields. Header controls were rejected above.
			quoted := strings.NewReplacer("\\", "\\\\", `"`, `\"`).Replace(part.Name)
			headers.Set("Content-Disposition", `form-data; name="`+quoted+`"`)
		} else {
			headers.Set("Content-Disposition", multipart.FileContentDisposition(part.Name, info.Filename))
		}
		headers.Set("Content-Type", info.ContentType)
		destination, err := writer.CreatePart(headers)
		if err != nil {
			return nil, multipartWriteError(ctx, err)
		}
		if part.Text != nil {
			if int64(len(*part.Text)) > limit-bounded.bytes {
				return nil, errMultipartLimit
			}
			if _, err := io.WriteString(destination, *part.Text); err != nil {
				return nil, multipartWriteError(ctx, err)
			}
			sum := sha256.Sum256([]byte(*part.Text))
			info.Bytes, info.SHA256 = int64(len(*part.Text)), hex.EncodeToString(sum[:])
		} else {
			info.Bytes, info.SHA256, err = snapshotMultipartFile(ctx, destination, part.File, limit-bounded.bytes)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, multipartWriteError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	u.bytes, u.sha256 = bounded.bytes, hex.EncodeToString(hash.Sum(nil))
	keep = true
	return u, nil
}

func multipartMetadata(part MultipartPart) (MultipartPartInfo, error) {
	if !multipartHeaderText(part.Name) || (part.Text != nil) == (part.File != "") {
		return MultipartPartInfo{}, errors.New("each multipart part requires a name and exactly one of text or file")
	}
	info := MultipartPartInfo{Name: part.Name, ContentType: part.ContentType}
	if part.Text != nil {
		if !utf8.ValidString(*part.Text) || part.Filename != "" {
			return MultipartPartInfo{}, errors.New("multipart text must be UTF-8 and cannot have a filename")
		}
		if info.ContentType == "" {
			info.ContentType = "text/plain; charset=utf-8"
		}
	} else {
		info.Filename = part.Filename
		if info.Filename == "" {
			info.Filename = filepath.Base(part.File)
		}
		if !multipartHeaderText(info.Filename) || strings.ContainsAny(info.Filename, "/\\") || info.Filename == "." || info.Filename == ".." {
			return MultipartPartInfo{}, errors.New("multipart filename must be a plain filename without directory components or control characters")
		}
		if info.ContentType == "" {
			info.ContentType = "application/octet-stream"
		}
	}
	media, parameters, err := mime.ParseMediaType(info.ContentType)
	if err != nil || !strings.Contains(media, "/") || strings.Contains(media, "*") || !multipartHeaderText(info.ContentType) {
		return MultipartPartInfo{}, errors.New("multipart contentType must be a concrete media type without control characters")
	}
	if part.Text != nil {
		charset := strings.ToLower(parameters["charset"])
		if charset != "" && charset != "utf-8" && charset != "us-ascii" {
			return MultipartPartInfo{}, errors.New("multipart text supports only UTF-8 or US-ASCII charsets")
		}
		if charset == "us-ascii" {
			for _, b := range []byte(*part.Text) {
				if b >= utf8.RuneSelf {
					return MultipartPartInfo{}, errors.New("multipart US-ASCII text contains non-ASCII bytes")
				}
			}
		}
	}
	return info, nil
}

func multipartHeaderText(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) {
		return false
	}
	for _, b := range []byte(value) {
		if b < 0x20 || b == 0x7f {
			return false
		}
	}
	return true
}

func snapshotMultipartFile(ctx context.Context, destination io.Writer, path string, limit int64) (int64, string, error) {
	if limit <= 0 {
		return 0, "", errMultipartLimit
	}
	source, err := SnapshotUpload(ctx, path, limit)
	if err != nil {
		return 0, "", err
	}
	defer source.Close()
	reader, err := source.Open(ctx)
	if err != nil {
		return 0, "", err
	}
	defer reader.Close()
	n, err := io.CopyBuffer(destination, reader, make([]byte, 64<<10))
	if err != nil {
		return 0, "", multipartWriteError(ctx, err)
	}
	if n != source.Bytes() {
		return 0, "", errors.New("multipart source snapshot is incomplete")
	}
	return n, source.SHA256(), nil
}

var errMultipartLimit = errors.New("multipart upload including framing exceeds size limit")

type multipartSink struct {
	ctx          context.Context
	writer       io.Writer
	limit, bytes int64
}

func (w *multipartSink) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.limit-w.bytes {
		return 0, errMultipartLimit
	}
	n, err := w.writer.Write(p)
	w.bytes += int64(n)
	return n, err
}

func multipartWriteError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, errMultipartLimit) {
		return errMultipartLimit
	}
	return errors.New("could not write multipart snapshot")
}
