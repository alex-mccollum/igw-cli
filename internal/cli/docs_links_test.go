package cli

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// Scan authored documentation, excluding generated binaries and run packets.
func documentationFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"docs", "internal", ".github"} {
		if err := filepath.WalkDir(filepath.Join("../..", dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".md") {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

var markdownLink = regexp.MustCompile(`\[[^\]\n]*\]\(([^\s)]+)\)`)
var inlineCode = regexp.MustCompile("`[^`]*`")

// This checks the inline links and ATX headings used by repository docs.
// External URLs remain review evidence, never network dependencies of tests.
func markdownLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if fence == "" {
				fence = trimmed[:3]
			} else if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			lines[i] = ""
		} else if fence != "" {
			lines[i] = ""
		}
	}
	return lines
}

func markdownAnchors(raw string) map[string]bool {
	anchors := map[string]bool{}
	for _, line := range markdownLines(raw) {
		line = strings.TrimSpace(line)
		heading := strings.TrimLeft(line, "#")
		if heading == line || !strings.HasPrefix(heading, " ") {
			continue
		}
		heading = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(heading), "#"))
		base := strings.Map(func(r rune) rune {
			switch {
			case r == ' ':
				return '-'
			case unicode.IsLetter(r), unicode.IsNumber(r), r == '-', r == '_':
				return unicode.ToLower(r)
			default:
				return -1
			}
		}, heading)
		anchor := base
		for n := 1; anchors[anchor]; n++ {
			anchor = fmt.Sprintf("%s-%d", base, n)
		}
		anchors[anchor] = true
	}
	return anchors
}

func checkMarkdownLink(source, target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return err
	}
	if u.IsAbs() || u.Host != "" {
		return nil
	}
	path := source
	if u.Path != "" {
		path = filepath.Join(filepath.Dir(source), filepath.FromSlash(u.Path))
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if u.Fragment != "" && strings.HasSuffix(path, ".md") {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !markdownAnchors(string(raw))[u.Fragment] {
			return fmt.Errorf("missing heading #%s in %s", u.Fragment, path)
		}
	}
	return nil
}

func TestDocsLinksResolve(t *testing.T) {
	for _, path := range documentationFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for line, text := range markdownLines(string(raw)) {
			for _, match := range markdownLink.FindAllStringSubmatch(inlineCode.ReplaceAllString(text, ""), -1) {
				if err := checkMarkdownLink(path, match[1]); err != nil {
					t.Errorf("%s:%d: %s: %v", path, line+1, match[1], err)
				}
			}
		}
	}
}

func TestDocLinksRejectMissingFilesAndHeadings(t *testing.T) {
	file := filepath.Join(t.TempDir(), "guide.md")
	if err := os.WriteFile(file, []byte("# Guide\n## Token `name:key`\n## Guide\n```md\n## Hidden\n```\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"missing.md", "#missing", "#hidden"} {
		if err := checkMarkdownLink(file, target); err == nil {
			t.Errorf("accepted broken link %s", target)
		}
	}
	for _, target := range []string{"#guide", "guide.md#guide-1", "#token-namekey", "https://example.invalid/missing.md#missing"} {
		if err := checkMarkdownLink(file, target); err != nil {
			t.Errorf("rejected link %s: %v", target, err)
		}
	}
}
