package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

const batchInputSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array","minItems":1,"maxItems":100,"items":{"type":"object","required":["id","operation"],"additionalProperties":false,"properties":{"id":{"type":"string","pattern":"^[A-Za-z0-9_.-]{1,64}$"},"operation":{"type":"string","minLength":1},"pathParams":{"type":"object","propertyNames":{"minLength":1},"additionalProperties":{"type":"string"}},"query":{"$ref":"#/$defs/values"},"headers":{"$ref":"#/$defs/values"},"body":true,"bodyText":{"type":"string"},"contentType":{"type":"string","minLength":1}},"not":{"required":["body","bodyText"]}},"$defs":{"values":{"type":"object","propertyNames":{"minLength":1},"additionalProperties":{"type":"array","items":{"type":"string"}}}}}`

func (i *invocation) batchCommand() *cobra.Command {
	var input string
	var options execute.BatchOptions
	cmd := &cobra.Command{Use: "batch", Short: "Run bounded independent requests with ordered per-item results", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&input, "input", "", "JSON item array, @file, or - for stdin (1 MiB, 100 items maximum)")
	_ = cmd.MarkFlagRequired("input")
	_ = f.SetAnnotation("input", inputSchemaAnnotation, []string{batchInputSchema})
	f.BoolVar(&options.DryRun, "dry-run", false, "Preview every item without sending operation requests")
	f.BoolVar(&options.Yes, "yes", false, "Confirm all mutating items in this batch")
	f.BoolVar(&options.ContinueOnError, "continue-on-error", false, "Continue independent items after ordinary failures; uncertainty, auth, and cancellation always stop")
	cmd.MarkFlagsMutuallyExclusive("yes", "dry-run")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		raw, err := i.readInput(input, execute.MaxBatchInputBytes)
		if err != nil {
			return err
		}
		items, err := decodeBatch(raw)
		if err != nil {
			return err
		}
		target, token, err := i.runtime()
		if err != nil {
			return err
		}
		service, err := i.service()
		if err != nil {
			return err
		}
		ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
		if err != nil {
			return err
		}
		defer cancel()
		options.Policy = catalog.Policy{Offline: i.offline, AllowStale: i.allowStale, Pin: i.pin}
		engine := execute.Engine{Catalog: service, HTTP: i.app.HTTP}
		i.output = engine.Batch(ctx, target, token, items, options)
		return nil
	}
	return cmd
}

// Decode exact names and values before any Gateway access. Bodies retain their
// original JSON bytes, including null and integers beyond float64 precision.
func decodeBatch(raw []byte) ([]execute.BatchItem, error) {
	bad := result.Usage("batch input must match the api batch --input schema; use schema --json and the command guide")
	if len(raw) > execute.MaxBatchInputBytes || jsonvalue.Validate(raw) != nil || !jsonvalue.ValidUnicode(raw) {
		return nil, bad
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 || len(rows) > execute.MaxBatchItems {
		return nil, bad
	}
	items := make([]execute.BatchItem, len(rows))
	for n, row := range rows {
		_, body := row["body"]
		_, text := row["bodyText"]
		if body && text {
			return nil, bad
		}
		r := &items[n].Request
		for key, value := range row {
			switch key {
			case "id", "operation", "contentType", "bodyText":
				var v *string
				if json.Unmarshal(value, &v) != nil || v == nil || key != "bodyText" && *v == "" {
					return nil, bad
				}
				switch key {
				case "id":
					items[n].ID = *v
				case "operation":
					r.Operation = *v
				case "contentType":
					r.ContentType = *v
				case "bodyText":
					r.Body = []byte(*v)
				}
			case "body":
				r.Body = append([]byte(nil), value...)
			case "pathParams":
				var values map[string]*string
				if json.Unmarshal(value, &values) != nil || values == nil {
					return nil, bad
				}
				r.PathParams = make(map[string]string, len(values))
				for k, v := range values {
					if k == "" || v == nil {
						return nil, bad
					}
					r.PathParams[k] = *v
				}
			case "query", "headers":
				var values map[string][]*string
				if json.Unmarshal(value, &values) != nil || values == nil {
					return nil, bad
				}
				out := make(map[string][]string, len(values))
				for k, entries := range values {
					if k == "" || entries == nil {
						return nil, bad
					}
					out[k] = make([]string, len(entries))
					for j, v := range entries {
						if v == nil {
							return nil, bad
						}
						out[k][j] = *v
					}
				}
				if key == "query" {
					r.Query = out
				} else {
					r.Headers = out
				}
			default:
				return nil, bad
			}
		}
	}
	if err := execute.ValidateBatch(items); err != nil {
		return nil, err
	}
	return items, nil
}
