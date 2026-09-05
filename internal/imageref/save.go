package imageref

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
)

// Save creates a new private directory, preserving existing candidates. The
// receipt is published last; its absence identifies an incomplete output.
// Consumers must still check its digests and qualify the referenced image.
func (c Candidate) Save(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := c.ConfigurationDigest(); err != nil {
		return err
	}
	receipt, err := json.MarshalIndent(c.Resolution, "", "  ")
	if err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	for _, file := range []struct {
		name string
		data []byte
	}{
		{"index.json", c.Index}, {"manifest.json", c.Manifest}, {"resolution.json", append(receipt, '\n')},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := saveFile(filepath.Join(dir, file.name), file.data); err != nil {
			return err
		}
	}
	return nil
}

func saveFile(path string, data []byte) error {
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
