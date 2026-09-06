package execute

import (
	"context"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

const (
	MaxBatchItems         = 100
	MaxBatchInputBytes    = 1 << 20
	MaxBatchResponseBytes = 256 << 10
)

type BatchItem struct {
	ID      string
	Request Request
}

type BatchOptions struct {
	DryRun, Yes, ContinueOnError bool
	Policy                       catalog.Policy
}

type BatchItemResult struct {
	ID     string        `json:"id"`
	Result result.Result `json:"result"`
}

type BatchReport struct {
	Items     []BatchItemResult `json:"items"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	NotRun    int               `json:"notRun"`
}

// ValidateBatch checks manifest structure without discovery or operation I/O.
// Operation-specific validation remains part of each ordered execution step.
func ValidateBatch(items []BatchItem) error {
	if len(items) == 0 || len(items) > MaxBatchItems {
		return result.Usage("batch requires 1..100 items")
	}
	seen := make(map[string]bool)
	remaining := MaxBatchInputBytes
	consume := func(value string) bool {
		if len(value) > remaining || !utf8.ValidString(value) {
			return false
		}
		remaining -= len(value)
		return true
	}
	for _, item := range items {
		if !validBatchID(item.ID) || seen[item.ID] {
			return result.Usage("batch IDs must be unique, 1..64 characters, and use only ASCII letters, digits, dot, underscore, or hyphen")
		}
		seen[item.ID] = true
		r := item.Request
		if r.Operation == "" || r.Method != "" || r.Path != "" || r.Upload != nil || r.Out != "" || r.Overwrite || r.Yes || r.DryRun || r.Offline || r.AllowStale || r.Pin != "" {
			return result.Usage("batch items require catalog operations and inline bodies; target, confirmation, and catalog policies belong to the invocation")
		}
		if r.MaxBodyBytes < 0 || r.MaxBodyBytes > MaxBatchResponseBytes {
			return result.Usage("batch responses are limited to 256 KiB per item")
		}
		if len(r.Body) > remaining {
			return result.Usage("batch input exceeds 1 MiB")
		}
		remaining -= len(r.Body)
		if !consume(item.ID) || !consume(r.Operation) || !consume(r.ContentType) {
			return result.Usage("batch input exceeds its size or UTF-8 limits")
		}
		for key, value := range r.PathParams {
			if key == "" || !consume(key) || !consume(value) {
				return result.Usage("invalid batch path inputs")
			}
		}
		for _, values := range []map[string][]string{r.Query, r.Headers} {
			for key, entries := range values {
				if key == "" || !consume(key) {
					return result.Usage("invalid batch input names")
				}
				for n, value := range entries {
					// Serialization repeats the name for every value. Count it
					// before constructing URLs or individual HTTP header lines.
					if n > 0 && !consume(key) || !consume(value) {
						return result.Usage("batch input exceeds its size or UTF-8 limits")
					}
				}
			}
		}
	}
	return nil
}

func validBatchID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

// Batch executes independent catalog operations serially against one snapshot.
// It never replays a mutation or rolls back earlier successful requests.
func (e Engine) Batch(ctx context.Context, target catalog.Target, token string, items []BatchItem, options BatchOptions) result.Result {
	if err := ValidateBatch(items); err != nil {
		return result.Failure(err)
	}
	if options.Yes && options.DryRun {
		return result.Failure(result.Usage("choose batch --yes or --dry-run"))
	}
	report := BatchReport{Items: make([]BatchItemResult, len(items)), NotRun: len(items)}
	for n, item := range items {
		report.Items[n] = BatchItemResult{ID: item.ID, Result: result.Result{Version: result.Version, Outcome: "not_run"}}
	}
	failBeforeRun := func(err error) result.Result {
		out := result.Failure(err)
		out.Data = report
		return out
	}
	if options.Policy.Offline && !options.DryRun {
		return failBeforeRun(result.Usage("offline batches require --dry-run"))
	}
	options.Policy.ForWrite = options.Yes && !options.DryRun
	scope, err := e.Open(ctx, target, token, options.Policy)
	if err != nil {
		return failBeforeRun(err)
	}
	defer scope.Close()
	meta := result.Metadata{Target: &target, Catalog: &scope.snapshot.Metadata, Stale: scope.snapshot.Stale, Warnings: append([]string(nil), scope.snapshot.Warnings...)}
	// Missing invocation confirmation must not allow an earlier read to run
	// before a known mutation is refused. Other item errors retain their order.
	if !options.Yes && !options.DryRun {
		for _, item := range items {
			op, err := scope.snapshot.Catalog.Resolve(item.Request.Operation)
			if err == nil && mutating(op.Method) {
				out := failBeforeRun(result.Usage("batch contains mutations; use --dry-run to review or --yes to execute"))
				out.Meta = meta
				return out
			}
		}
	}
	var firstFailure *result.Problem
	accepted, uncertain := false, false
	for n, item := range items {
		request := item.Request
		request.Yes, request.DryRun = options.Yes, options.DryRun
		if request.MaxBodyBytes == 0 {
			request.MaxBodyBytes = MaxBatchResponseBytes
		}
		step := scope.Run(request)
		if step.Meta.Catalog == nil {
			step.Meta = meta
		}
		report.Items[n].Result = step
		report.NotRun--
		if step.OK {
			report.Succeeded++
			accepted = accepted || step.Outcome == "accepted"
			continue
		}
		report.Failed++
		if firstFailure == nil {
			firstFailure = step.Error
		}
		uncertain = step.Outcome == "uncertain"
		terminal := uncertain || step.Error == nil || step.Error.Code == 6 || step.Error.Kind == "canceled" || step.Error.Kind == "timeout" || ctx.Err() != nil
		if terminal {
			firstFailure = step.Error
		}
		if terminal || !options.ContinueOnError {
			break
		}
	}
	out := result.Success(report)
	out.Meta = meta
	if report.Failed != 0 {
		out.OK, out.Outcome = false, "failed"
		if report.Succeeded != 0 {
			out.Outcome = "partial"
		}
		if uncertain {
			out.Outcome = "uncertain"
		}
		code := 7
		if firstFailure != nil {
			code = firstFailure.Code
		}
		out.Error = &result.Problem{Kind: "batch", Message: "batch has failed items; inspect per-item results before retrying", Code: code}
	} else if options.DryRun {
		out.Outcome = "preview"
	} else if accepted {
		out.Outcome, out.Meta.Verification = "accepted", "not_performed"
	}
	return out
}
