package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type jsonSelectOptions struct {
	compact   bool
	raw       bool
	selectors []string
}

func newJSONSelectOptions(jsonOutput, compact, raw bool, selectors []string) (jsonSelectOptions, error) {
	opts := jsonSelectOptions{
		compact: compact,
		raw:     raw,
	}

	normalized, err := normalizeJSONSelectors(selectors)
	if err != nil {
		return opts, err
	}
	opts.selectors = normalized

	if opts.compact && !jsonOutput {
		return opts, &igwerr.UsageError{Msg: "required: --json when using --compact"}
	}
	if opts.raw && !jsonOutput {
		return opts, &igwerr.UsageError{Msg: "required: --json when using --raw"}
	}
	if len(opts.selectors) > 0 && !jsonOutput {
		return opts, &igwerr.UsageError{Msg: "required: --json when using --select"}
	}
	if opts.raw && len(opts.selectors) != 1 {
		return opts, &igwerr.UsageError{Msg: "required: exactly one --select when using --raw"}
	}
	if opts.raw && opts.compact {
		return opts, &igwerr.UsageError{Msg: "cannot use --raw with --compact"}
	}

	return opts, nil
}

func normalizeJSONSelectors(selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return nil, nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(selectors))

	for _, raw := range selectors {
		selector := strings.TrimSpace(raw)
		if selector == "" {
			return nil, &igwerr.UsageError{Msg: "invalid --select value: empty selector"}
		}
		if strings.Contains(selector, ",") {
			return nil, &igwerr.UsageError{
				Msg: fmt.Sprintf("invalid --select value %q: commas are not supported; repeat --select for multiple selectors", selector),
			}
		}
		if _, exists := seen[selector]; exists {
			continue
		}
		seen[selector] = struct{}{}
		out = append(out, selector)
	}

	return out, nil
}

func printJSONSelection(w io.Writer, payload any, opts jsonSelectOptions) error {
	if opts.raw {
		extracted, err := extractJSONPathRaw(payload, opts.selectors[0])
		if err != nil {
			return &igwerr.UsageError{
				Msg: fmt.Sprintf("invalid --select path %q: %v", opts.selectors[0], err),
			}
		}
		if _, err := fmt.Fprintln(w, extracted); err != nil {
			return igwerr.NewTransportError(err)
		}
		return nil
	}

	if len(opts.selectors) > 0 {
		values, err := selectJSONPaths(payload, opts.selectors)
		if err != nil {
			return err
		}
		return writeJSONWithOptions(w, values, opts.compact)
	}

	return writeJSONWithOptions(w, payload, opts.compact)
}

func selectionErrorOptions(opts jsonSelectOptions) jsonSelectOptions {
	return jsonSelectOptions{compact: opts.compact}
}
