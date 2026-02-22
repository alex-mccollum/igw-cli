package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func loadAPIOperations(specFile string) ([]apidocs.Operation, error) {
	ops, resolvedSpecFile, candidates, err := loadAPIOperationsRaw(specFile)
	if err != nil {
		return nil, openAPILoadError(resolvedSpecFile, candidates, err)
	}
	return ops, nil
}

func loadAPIOperationsRaw(specFile string) ([]apidocs.Operation, string, []string, error) {
	resolvedSpecFile, candidates := resolveSpecFile(specFile)
	ops, err := apidocs.LoadOperations(resolvedSpecFile)
	return ops, resolvedSpecFile, candidates, err
}

func openAPILoadError(resolvedSpecFile string, candidates []string, err error) error {
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if len(candidates) > 1 {
				return &igwerr.UsageError{
					Msg: fmt.Sprintf("OpenAPI spec not found. checked: %q (cwd), %q (config). pass --spec-file /path/to/openapi.json", candidates[0], candidates[1]),
				}
			}
			return &igwerr.UsageError{
				Msg: fmt.Sprintf("OpenAPI spec not found at %q (pass --spec-file /path/to/openapi.json)", resolvedSpecFile),
			}
		}
		return &igwerr.UsageError{Msg: err.Error()}
	}
	return nil
}

func formatOperationMatches(ops []apidocs.Operation) string {
	if len(ops) == 0 {
		return ""
	}

	limit := len(ops)
	if limit > 3 {
		limit = 3
	}

	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%s %s", ops[i].Method, ops[i].Path))
	}
	if len(ops) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(ops)-limit))
	}

	return strings.Join(parts, "; ")
}

func resolveSpecFile(specFile string) (string, []string) {
	specFile = strings.TrimSpace(specFile)
	if specFile != "" && specFile != apidocs.DefaultSpecFile {
		return specFile, []string{specFile}
	}

	candidates := []string{apidocs.DefaultSpecFile}
	if cfgDir, err := config.Dir(); err == nil {
		candidates = append(candidates, filepath.Join(cfgDir, apidocs.DefaultSpecFile))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, candidates
		}
	}

	return candidates[0], candidates
}
