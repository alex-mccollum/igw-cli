package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

// Libopenapi accepts YAML, including a block mapping whose values are JSON.
// Its YAML scanner can queue a whole flow-style root as a possible complex
// key. A block root lets it consume each value without retaining that queue.
// JSON encoding preserves scalar spellings; indentation also prevents the
// node index from doing quadratic work on one huge compact line.
// This private representation never replaces the vendor document or hashes.
func modelDocument(value any) ([]byte, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("OpenAPI root must be an object")
	}
	names := make([]string, 0, len(root))
	for name := range root {
		names = append(names, name)
	}
	sort.Strings(names)
	var output bytes.Buffer
	for _, name := range names {
		key, _ := json.Marshal(name)
		encoded, err := json.MarshalIndent(root[name], "  ", "  ")
		if err != nil {
			return nil, err
		}
		output.Write(key)
		output.WriteString(": ")
		output.Write(encoded)
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}
