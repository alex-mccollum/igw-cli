package apidocs

import (
	"encoding/json"
	"fmt"
	"os"
)

type operationIndexFile struct {
	SpecPath        string      `json:"specPath"`
	SpecSize        int64       `json:"specSize"`
	SpecModUnixNano int64       `json:"specModUnixNano"`
	Operations      []Operation `json:"operations"`
}

func IndexPathForSpec(specPath string) string {
	return specPath + ".index.json"
}

func LoadOperationIndex(indexPath string, specPath string, specSize int64, specModUnixNano int64) ([]Operation, error) {
	b, err := os.ReadFile(indexPath) //nolint:gosec // index path derived from spec file path
	if err != nil {
		return nil, err
	}

	var index operationIndexFile
	if err := json.Unmarshal(b, &index); err != nil {
		return nil, fmt.Errorf("parse operation index %q: %w", indexPath, err)
	}
	if index.SpecPath != specPath || index.SpecSize != specSize || index.SpecModUnixNano != specModUnixNano {
		return nil, os.ErrNotExist
	}

	if len(index.Operations) == 0 {
		return nil, os.ErrNotExist
	}
	out := make([]Operation, len(index.Operations))
	copy(out, index.Operations)
	return out, nil
}

func WriteOperationIndex(indexPath string, specPath string, specSize int64, specModUnixNano int64, ops []Operation) error {
	if len(ops) == 0 {
		return nil
	}

	payload := operationIndexFile{
		SpecPath:        specPath,
		SpecSize:        specSize,
		SpecModUnixNano: specModUnixNano,
		Operations:      make([]Operation, len(ops)),
	}
	copy(payload.Operations, ops)

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode operation index: %w", err)
	}

	tmp := indexPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write operation index temp: %w", err)
	}
	if err := os.Rename(tmp, indexPath); err != nil {
		return fmt.Errorf("commit operation index: %w", err)
	}
	return nil
}
