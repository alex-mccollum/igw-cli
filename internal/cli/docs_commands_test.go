package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Parse examples without executing handlers, opening files, or contacting a
// Gateway. Command paths, flags, literal flag types, and arity come from Cobra.
func TestDocsCommandsMatchCommandTree(t *testing.T) {
	count := 0
	for _, name := range documentationFiles(t) {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		inCode := false
		var logical string
		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		for line := 1; scanner.Scan(); line++ {
			text := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(text, "```") {
				inCode = !inCode
				continue
			}
			if !inCode {
				continue
			}
			if logical == "" && !strings.HasPrefix(text, "igw ") && !strings.HasPrefix(text, "bin/igw ") {
				continue
			}
			logical += text
			if strings.HasSuffix(logical, "\\") {
				logical = strings.TrimSuffix(logical, "\\") + " "
				continue
			}
			args, err := exampleWords(logical)
			logical = ""
			if err != nil {
				t.Errorf("%s:%d: %v", name, line, err)
				continue
			}
			if len(args) < 2 {
				t.Errorf("%s:%d: empty command", name, line)
				continue
			}
			count++
			if err := validateExample(args[1:]); err != nil {
				t.Errorf("%s:%d: %v", name, line, err)
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if logical != "" {
			t.Errorf("%s: unterminated continuation", name)
		}
	}
	if count < 30 {
		t.Fatalf("expected substantial canonical coverage, checked %d commands", count)
	}
}

func validateExample(args []string) error {
	i := &invocation{}
	root := i.commands()
	root.InitDefaultCompletionCmd()
	cmd, rest, err := root.Find(args)
	if err != nil {
		return err
	}
	cmd.InitDefaultHelpFlag()
	if cmd == root && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		return fmt.Errorf("unknown command %s", rest[0])
	}
	// Shell variables have no literal value during static validation. Preserve
	// their flag presence and validate the type using the flag's own default.
	for n := 0; n < len(rest); n++ {
		name, value, equals := strings.Cut(rest[n], "=")
		if !strings.HasPrefix(name, "--") {
			continue
		}
		flag := cmd.Flags().Lookup(strings.TrimPrefix(name, "--"))
		if flag == nil {
			flag = cmd.InheritedFlags().Lookup(strings.TrimPrefix(name, "--"))
		}
		if flag == nil {
			continue
		} // ParseFlags supplies the actual unknown-flag error.
		if !equals && flag.NoOptDefVal == "" && n+1 < len(rest) {
			n++
			value = rest[n]
		}
		if strings.Contains(value, "$") && flag.Value.Type() != "string" && flag.Value.Type() != "stringArray" {
			if equals {
				rest[n] = name + "=" + flag.DefValue
			} else {
				rest[n] = flag.DefValue
			}
		}
	}
	if err := cmd.ParseFlags(rest); err != nil {
		return err
	}
	if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
		return err
	}
	return cmd.ValidateRequiredFlags()
}

// This recognizes quoting and continuations used in documented shell commands;
// it intentionally does not expand variables or execute shell substitutions.
func exampleWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	flush := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	for _, r := range line {
		if escaped {
			word.WriteRune(r)
			started = true
			escaped = false
			continue
		}
		if quote == '\'' {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\\' {
			escaped = true
			started = true
			continue
		}
		if quote == '"' {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if strings.ContainsRune("<>|&;", r) || r == '#' && !started {
			break
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unterminated shell quoting")
	}
	flush()
	return words, nil
}

func TestDocValidationRejectsDriftWithoutExecuting(t *testing.T) {
	for _, text := range []string{"igw rpc", "igw api unknown", "igw resource update only-one", "igw profile set dev --unknown", "igw logs list --limit invalid"} {
		args, err := exampleWords(text)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateExample(args[1:]); err == nil {
			t.Fatalf("accepted stale example: %s", text)
		}
	}
	for _, text := range []string{`igw api describe 'GET /items/{name}' --json`, `igw profile set dev --url "https://gateway.invalid/base" --yes`, `igw logs list --limit "$LIMIT" --json | jq '.data'`} {
		args, err := exampleWords(text)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateExample(args[1:]); err != nil {
			t.Fatalf("valid example refused: %v", err)
		}
	}
}
