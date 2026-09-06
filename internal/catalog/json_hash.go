package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
)

// Encoder uses the same canonical representation as Marshal. Withhold its
// final newline so existing document and contract identities stay unchanged,
// without copying a large encoded document into another byte slice.
func canonicalDigest(value any, prefix string) (string, error) {
	h := sha256.New()
	_, _ = h.Write([]byte(prefix))
	w := &jsonHashWriter{hash: h}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type jsonHashWriter struct {
	hash    hash.Hash
	last    [1]byte
	pending bool
}

func (w *jsonHashWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.pending {
		_, _ = w.hash.Write(w.last[:])
	}
	_, _ = w.hash.Write(p[:len(p)-1])
	w.last[0], w.pending = p[len(p)-1], true
	return len(p), nil
}
