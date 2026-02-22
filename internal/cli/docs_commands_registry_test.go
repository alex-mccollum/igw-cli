package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDocsCommandsUseKnownRegistryCommandShapes(t *testing.T) {
	t.Parallel()

	repoRoot, err := repoRootFromWD()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	docPath := filepath.Join(repoRoot, "docs", "commands.md")
	shapes, err := extractCommandShapesFromDoc(docPath)
	if err != nil {
		t.Fatalf("extract docs command shapes: %v", err)
	}
	if len(shapes) == 0 {
		t.Fatalf("no igw command examples found in %s", docPath)
	}

	allowed := allowedCommandShapes()

	var unknown []string
	for _, shape := range shapes {
		if _, ok := allowed[shape]; !ok {
			unknown = append(unknown, shape)
		}
	}

	if len(unknown) > 0 {
		sort.Strings(unknown)
		t.Fatalf("docs contain command shapes not present in CLI registry: %s", strings.Join(unknown, ", "))
	}
}

func repoRootFromWD() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}

func extractCommandShapesFromDoc(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	shapeSet := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "igw ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "igw" {
			continue
		}

		shapeParts := make([]string, 0, 3)
		for _, tok := range fields[1:] {
			if tok == `\` || strings.HasPrefix(tok, "--") {
				break
			}
			shapeParts = append(shapeParts, tok)
			if len(shapeParts) == 3 {
				break
			}
		}
		if len(shapeParts) == 0 {
			continue
		}

		shapeSet[strings.Join(shapeParts, " ")] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	shapes := make([]string, 0, len(shapeSet))
	for shape := range shapeSet {
		shapes = append(shapes, shape)
	}
	sort.Strings(shapes)
	return shapes, nil
}

func allowedCommandShapes() map[string]struct{} {
	allowed := make(map[string]struct{})

	for _, cmd := range rootCommands {
		allowed[cmd.Name] = struct{}{}
		for _, sub := range cmd.Subcommands {
			allowed[cmd.Name+" "+sub] = struct{}{}
		}
	}

	for chain, subs := range nestedCompletionCommands {
		allowed[chain] = struct{}{}
		for _, sub := range subs {
			allowed[chain+" "+sub] = struct{}{}
		}
	}

	// runCompletion currently supports one positional shell argument.
	allowed["completion bash"] = struct{}{}

	return allowed
}
