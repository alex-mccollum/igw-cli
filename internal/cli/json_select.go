package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type jsonSelectOptions struct {
	compact bool
	field   string
	fields  []string
}

func newJSONSelectOptions(jsonOutput, compact bool, fieldPath, fieldsCSV string) (jsonSelectOptions, error) {
	opts := jsonSelectOptions{
		compact: compact,
		field:   strings.TrimSpace(fieldPath),
	}

	fields, err := parseJSONFieldsCSV(fieldsCSV)
	if err != nil {
		return opts, err
	}
	opts.fields = fields

	if opts.compact && !jsonOutput {
		return opts, &igwerr.UsageError{Msg: "required: --json when using --compact"}
	}
	if opts.field != "" && !jsonOutput {
		return opts, &igwerr.UsageError{Msg: "required: --json when using --field"}
	}
	if len(opts.fields) > 0 && !jsonOutput {
		return opts, &igwerr.UsageError{Msg: "required: --json when using --fields"}
	}
	if opts.field != "" && len(opts.fields) > 0 {
		return opts, &igwerr.UsageError{Msg: "use only one of --field or --fields"}
	}

	return opts, nil
}

func parseJSONFieldsCSV(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))

	for _, part := range parts {
		selector := strings.TrimSpace(part)
		if selector == "" {
			return nil, &igwerr.UsageError{Msg: "invalid --fields list: empty selector"}
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
	if opts.field != "" {
		extracted, err := extractJSONField(payload, opts.field)
		if err != nil {
			return &igwerr.UsageError{
				Msg: fmt.Sprintf("invalid --field path %q: %v", opts.field, err),
			}
		}
		if _, err := fmt.Fprintln(w, extracted); err != nil {
			return igwerr.NewTransportError(err)
		}
		return nil
	}

	if len(opts.fields) > 0 {
		values, err := selectJSONFields(payload, opts.fields)
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
