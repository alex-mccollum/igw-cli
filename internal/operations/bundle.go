package operations

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type Runner interface {
	Run(execute.Request) result.Result
}

type BundleStatus struct {
	State        string `json:"state"`
	GatewayState string `json:"gatewayState"`
	FileSize     int64  `json:"fileSize"`
}

type BundleRequest struct {
	Generate, Yes, DryRun bool
	Out                   string
	Overwrite             bool
	MaxBytes              int64
	Interval              time.Duration
}

type BundleEvidence struct {
	Before                 BundleStatus     `json:"before"`
	Last                   BundleStatus     `json:"last"`
	Polls                  int              `json:"polls"`
	GenerationAcknowledged bool             `json:"generationAcknowledged"`
	GenerationState        string           `json:"generationState,omitempty"`
	Correlation            string           `json:"correlation"`
	Request                *execute.Preview `json:"request,omitempty"`
}

func ParseBundleStatus(raw []byte) (BundleStatus, error) {
	if jsonvalue.Validate(raw) != nil {
		return BundleStatus{}, bundleProblem("response", "invalid diagnostics status")
	}
	var object map[string]json.RawMessage
	var state *string
	var size *int64
	if json.Unmarshal(raw, &object) != nil || json.Unmarshal(object["state"], &state) != nil || json.Unmarshal(object["fileSize"], &size) != nil || state == nil || size == nil || *size < 0 {
		return BundleStatus{}, bundleProblem("response", "diagnostics status requires a state and nonnegative file size")
	}
	status := BundleStatus{GatewayState: *state, FileSize: *size}
	switch *state {
	case "Invalid":
		status.State = "empty"
	case "Generating":
		status.State = "generating"
	case "Valid":
		status.State = "ready"
		if *size == 0 {
			return BundleStatus{}, bundleProblem("response", "ready diagnostics bundle has no bytes")
		}
	default:
		return BundleStatus{}, bundleProblem("response", "Gateway returned an unqualified diagnostics state")
	}
	return status, nil
}

func BundleState(runner Runner) result.Result {
	out := runner.Run(execute.Request{Operation: "GET /data/api/v1/diagnostics/bundle/status"})
	if !out.OK {
		return out
	}
	raw, _ := out.Data.(json.RawMessage)
	state, err := ParseBundleStatus(raw)
	if err != nil {
		out.OK, out.Outcome, out.Data, out.Error = false, "failed", nil, result.FromError(err)
		return out
	}
	out.Data = state
	return out
}

func (r BundleRequest) Validate() error {
	if r.Out == "" || r.MaxBytes <= 0 {
		return result.Usage("diagnostics download requires --out and a positive --max-bytes")
	}
	if r.Generate {
		if !r.Yes && !r.DryRun {
			return result.Usage("diagnostics collection requires --yes; inspect --dry-run first")
		}
		if r.Interval < 100*time.Millisecond || r.Interval > time.Minute {
			return result.Usage("poll interval must be between 100ms and 1m")
		}
	} else if r.DryRun {
		return result.Usage("download does not generate a bundle; use collect --dry-run to preview generation")
	}
	return nil
}

// Bundle requests generation when selected, observes the Gateway's latest
// bundle, and publishes only after its reported size and ready state agree
// with a complete private download. The API offers no job correlation ID.
func Bundle(ctx context.Context, runner Runner, input BundleRequest) result.Result {
	if err := input.Validate(); err != nil {
		return result.Failure(err)
	}
	stateResult := BundleState(runner)
	if !stateResult.OK {
		return stateResult
	}
	state := stateResult.Data.(BundleStatus)
	evidence := BundleEvidence{Before: state, Last: state, Correlation: "gateway_latest"}
	if input.Generate && state.State == "generating" {
		return bundleFailure(stateResult, evidence, bundleProblem("conflict", "diagnostics generation is already running; inspect status or download when ready"), false)
	}
	if !input.Generate && state.State != "ready" {
		return bundleFailure(stateResult, evidence, bundleProblem("not_ready", "no ready diagnostics bundle is available; use collect to generate one"), false)
	}
	request := execute.Request{Operation: "POST /data/api/v1/diagnostics/bundle/generate", Yes: input.Yes, DryRun: input.DryRun}
	if input.DryRun {
		out := runner.Run(request)
		if out.OK {
			preview, ok := out.Data.(execute.Preview)
			if !ok {
				return bundleFailure(out, evidence, bundleProblem("response", "diagnostics preview unavailable"), false)
			}
			evidence.Request = &preview
			out.Data = evidence
		}
		return out
	}
	// Reserve output before starting a job, but never during a preview.
	w, err := artifact.New(input.Out, input.Overwrite)
	if err != nil {
		return result.Failure(result.Usage("cannot create diagnostics output; an existing file requires --overwrite"))
	}
	defer w.Abort()
	if input.Generate {
		out := runner.Run(request)
		if !out.OK {
			if out.Error != nil && out.Error.Code != 2 && out.Error.Code != 6 {
				return bundleFailure(out, evidence, out.Error, true)
			}
			out.Data = evidence
			return out
		}
		var object map[string]json.RawMessage
		var generationState string
		raw, _ := out.Data.(json.RawMessage)
		if jsonvalue.Validate(raw) != nil || json.Unmarshal(raw, &object) != nil || json.Unmarshal(object["state"], &generationState) != nil || (generationState != "Generating" && generationState != "Valid") {
			return bundleFailure(out, evidence, bundleProblem("response", "Gateway did not provide a qualified generation acknowledgement; inspect status before retrying"), true)
		}
		evidence.GenerationAcknowledged = true
		evidence.GenerationState = generationState
		for {
			stateResult = BundleState(runner)
			evidence.Polls++
			if !stateResult.OK {
				if !transientBundleRead(stateResult) {
					return bundleFailure(stateResult, evidence, stateResult.Error, true)
				}
			} else {
				state = stateResult.Data.(BundleStatus)
				evidence.Last = state
				if state.State == "ready" {
					break
				}
				if state.State != "generating" {
					return bundleFailure(stateResult, evidence, bundleProblem("generation", "diagnostics generation did not produce a valid bundle"), true)
				}
			}
			if err := waitPoll(ctx, input.Interval); err != nil {
				return bundleFailure(stateResult, evidence, err, true)
			}
		}
	}
	if state.FileSize > input.MaxBytes {
		return bundleFailure(stateResult, evidence, bundleProblem("limit", "diagnostics bundle exceeds --max-bytes"), input.Generate)
	}
	dir, err := os.MkdirTemp("", "igw-bundle-*")
	if err != nil {
		return bundleFailure(stateResult, evidence, bundleProblem("artifact", "cannot create private diagnostics download"), input.Generate)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "bundle.zip")
	out := runner.Run(execute.Request{Operation: "GET /data/api/v1/diagnostics/bundle/download", Out: path, MaxBodyBytes: input.MaxBytes})
	if !out.OK {
		return bundleFailure(out, evidence, out.Error, input.Generate)
	}
	if out.Artifact == nil || out.Artifact.Path != path || out.Artifact.Bytes != state.FileSize {
		return bundleFailure(out, evidence, bundleProblem("verification", "download size differs from the ready diagnostics bundle"), input.Generate)
	}
	after := BundleState(runner)
	if !after.OK {
		return bundleFailure(after, evidence, after.Error, input.Generate)
	}
	evidence.Last = after.Data.(BundleStatus)
	if evidence.Last.State != "ready" || evidence.Last.FileSize != state.FileSize {
		return bundleFailure(after, evidence, bundleProblem("verification", "diagnostics bundle changed during download; inspect current status"), input.Generate)
	}
	file, err := os.Open(path)
	if err != nil {
		return bundleFailure(out, evidence, bundleProblem("artifact", "private diagnostics download unavailable"), input.Generate)
	}
	defer file.Close()
	if copied, err := io.Copy(w, bundleReader{ctx, file}); err != nil {
		return bundleFailure(out, evidence, err, input.Generate)
	} else if copied != state.FileSize {
		return bundleFailure(out, evidence, bundleProblem("verification", "private diagnostics download changed before publication"), input.Generate)
	}
	if err := ctx.Err(); err != nil {
		return bundleFailure(out, evidence, err, input.Generate)
	}
	info, err := w.Commit()
	if err != nil {
		return bundleFailure(out, evidence, bundleProblem("artifact", "could not publish diagnostics bundle"), input.Generate)
	}
	out.Data, out.Artifact, out.Outcome, out.Meta.Verification = evidence, &info, "completed", "size_matched"
	out.Meta.Warnings = append(out.Meta.Warnings, "The Gateway exposes the latest bundle without a job ID or content digest. Size and ready state were checked; concurrent generation of a same-sized bundle cannot be distinguished.")
	if evidence.GenerationState == "Valid" {
		out.Meta.Warnings = append(out.Meta.Warnings, "The generation request returned an already-ready state. A new generation job is not proven.")
	}
	return out
}

func transientBundleRead(out result.Result) bool {
	if out.Error == nil {
		return false
	}
	if out.Error.Kind == "transport" {
		return true
	}
	if out.Error.Kind != "http" {
		return false
	}
	raw, _ := json.Marshal(out.Error.Details)
	var details struct {
		HTTPStatus int `json:"httpStatus"`
	}
	_ = json.Unmarshal(raw, &details)
	switch details.HTTPStatus {
	case 429, 502, 503, 504:
		return true
	}
	return false
}
func waitPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func bundleProblem(kind, message string) error {
	return &result.Problem{Kind: kind, Message: message, Code: 7}
}
func bundleFailure(out result.Result, evidence BundleEvidence, err error, generated bool) result.Result {
	if err == nil {
		err = bundleProblem("response", "diagnostics operation failed")
	}
	out.OK, out.Data, out.Artifact, out.Error, out.Outcome = false, evidence, nil, result.FromError(err), "failed"
	if out.Error == nil {
		out.Error = &result.Problem{Kind: "response", Message: "diagnostics operation failed", Code: 7}
	}
	out.Meta.Verification = "not_verified"
	if generated {
		out.Outcome = "uncertain"
		out.Meta.Warnings = append(out.Meta.Warnings, "Generation may continue on the Gateway. Inspect status before retrying; the CLI does not replay generation requests.")
	}
	return out
}

type bundleReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r bundleReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
