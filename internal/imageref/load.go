package imageref

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Load revalidates a saved candidate without network access. A receipt alone
// cannot establish provenance: both exact manifests and their link must agree.
func Load(ctx context.Context, dir string) (Candidate, error) {
	var candidate Candidate
	for _, file := range []struct {
		name   string
		limit  int64
		target *[]byte
	}{
		{"index.json", maxManifestBytes, &candidate.Index},
		{"manifest.json", maxManifestBytes, &candidate.Manifest},
	} {
		b, err := readFile(ctx, filepath.Join(dir, file.name), file.limit)
		if err != nil {
			return Candidate{}, err
		}
		*file.target = b
	}
	b, err := readFile(ctx, filepath.Join(dir, "resolution.json"), 1<<20)
	if err != nil {
		return Candidate{}, err
	}
	if decode(b, &candidate.Resolution) != nil {
		return Candidate{}, errors.New("invalid image resolution receipt")
	}
	if _, err := candidate.ConfigurationDigest(); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

// ConfigurationDigest validates all saved provenance before returning the
// platform image's config digest for comparison with Docker's observed image ID.
func (c Candidate) ConfigurationDigest() (string, error) {
	r := c.Resolution
	if len(c.Index) > maxManifestBytes || len(c.Manifest) > maxManifestBytes {
		return "", errors.New("image resolution manifest exceeds size limit")
	}
	if r.Version != 1 || r.Repository != Repository || !releaseTag.MatchString(r.Tag) || r.ResolvedAt.IsZero() || r.Image != Repository+"@"+r.IndexDigest || r.Platform != Platform {
		return "", errors.New("invalid image resolution provenance")
	}
	if err := verifyDigest(c.Index, r.IndexDigest); err != nil {
		return "", err
	}
	selected, _, err := selectPlatform(c.Index)
	if err != nil {
		return "", err
	}
	if selected.Digest != r.ManifestDigest {
		return "", errors.New("resolution receipt selects a different platform manifest")
	}
	return validatePlatformManifest(c.Manifest, selected)
}

func readFile(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("image resolution requires bounded regular files")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("image resolution file changed before reading")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("image resolution file exceeds size limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b, nil
}
