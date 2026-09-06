package operations

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

type RestartRunner interface {
	Runner
	Require(catalog.Capability) result.Result
}

type RestartRequest struct {
	Yes, DryRun bool
	Interval    time.Duration
}

type PendingRestartTasks struct {
	Pending []string `json:"pending"`
}

// Uptime uses the API's number without assigning undocumented units. Only
// a strict decrease is used as evidence; elapsed wall time is never inferred.
type RestartObservation struct {
	NodeID    string   `json:"nodeId"`
	ProcessID int64    `json:"processId"`
	Uptime    float64  `json:"uptime"`
	Pending   []string `json:"pending"`
}

type RestartEvidence struct {
	Before       RestartObservation `json:"before"`
	Last         RestartObservation `json:"last"`
	Polls        int                `json:"polls"`
	Acknowledged bool               `json:"acknowledged"`
	Proof        string             `json:"proof"`
	Correlation  string             `json:"correlation"`
	Request      *execute.Preview   `json:"request,omitempty"`
}

func (r RestartRequest) Validate() error {
	if r.Yes && r.DryRun {
		return result.Usage("choose --dry-run or --yes, not both")
	}
	if !r.Yes && !r.DryRun {
		return result.Usage("Gateway restart requires --yes; inspect --dry-run first")
	}
	if r.Interval < 100*time.Millisecond || r.Interval > time.Minute {
		return result.Usage("poll interval must be between 100ms and 1m")
	}
	return nil
}

func restartObject(out result.Result) (map[string]json.RawMessage, error) {
	raw, ok := out.Data.(json.RawMessage)
	var object map[string]json.RawMessage
	if !ok || jsonvalue.Validate(raw) != nil || !jsonvalue.ValidUnicode(raw) || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, restartProblem("response", "Gateway restart observation requires an unambiguous JSON object")
	}
	return object, nil
}

func RestartTasks(runner Runner) result.Result {
	out := runner.Run(execute.Request{Operation: workflow.RestartTasksOperation, MaxBodyBytes: 1 << 20})
	if !out.OK {
		return out
	}
	object, err := restartObject(out)
	var pending []*string
	if err == nil && (json.Unmarshal(object["pending"], &pending) != nil || pending == nil) {
		err = restartProblem("response", "restart tasks require a non-null array of strings")
	}
	tasks := make([]string, 0, len(pending))
	for _, task := range pending {
		if task == nil {
			err = restartProblem("response", "restart tasks require a non-null array of strings")
			break
		}
		tasks = append(tasks, *task)
	}
	if err != nil {
		out.OK, out.Outcome, out.Data, out.Error = false, "failed", nil, result.FromError(err)
	} else {
		out.Data = PendingRestartTasks{Pending: tasks}
	}
	return out
}

func restartNode(runner Runner) result.Result {
	out := runner.Run(execute.Request{Operation: workflow.RedundancyOperation, MaxBodyBytes: 1 << 20})
	if !out.OK {
		return out
	}
	object, err := restartObject(out)
	var id string
	if err == nil && (json.Unmarshal(object["localId"], &id) != nil || strings.TrimSpace(id) == "" || len(id) > 4096) {
		err = restartProblem("response", "Gateway redundancy response requires a nonempty localId for restart verification")
	}
	if err != nil {
		out.OK, out.Outcome, out.Data, out.Error = false, "failed", nil, result.FromError(err)
	} else {
		out.Data = id
	}
	return out
}

// The two node reads reject observed routing changes around overview/tasks.
// They cannot establish affinity or atomicity across separate HTTP requests.
func observeRestart(runner Runner) result.Result {
	first := restartNode(runner)
	if !first.OK {
		return first
	}
	out := runner.Run(execute.Request{Operation: workflow.OverviewOperation, MaxBodyBytes: 1 << 20})
	if !out.OK {
		return out
	}
	object, err := restartObject(out)
	var pid *int64
	var uptime *float64
	if err == nil && (json.Unmarshal(object["processId"], &pid) != nil || pid == nil || *pid <= 0 || json.Unmarshal(object["uptime"], &uptime) != nil || uptime == nil || math.Signbit(*uptime)) {
		err = restartProblem("response", "Gateway overview requires a positive integral processId and a nonnegative uptime")
	}
	if err != nil {
		out.OK, out.Outcome, out.Data, out.Error = false, "failed", nil, result.FromError(err)
		return out
	}
	tasks := RestartTasks(runner)
	if !tasks.OK {
		return tasks
	}
	last := restartNode(runner)
	if !last.OK {
		return last
	}
	if first.Data.(string) != last.Data.(string) {
		last.OK, last.Outcome, last.Data, last.Error = false, "failed", nil, restartProblem("identity", "Gateway reported localId changed during observation; select a direct node URL")
		return last
	}
	last.Data = RestartObservation{NodeID: first.Data.(string), ProcessID: *pid, Uptime: *uptime, Pending: tasks.Data.(PendingRestartTasks).Pending}
	return last
}

// Restart sends at most one POST. Every subsequent step is a bounded read.
// A new process on the observed node proves a restart was observed, not that
// this request exclusively caused it or that every module is healthy.
func Restart(ctx context.Context, runner RestartRunner, input RestartRequest) result.Result {
	if err := input.Validate(); err != nil {
		return result.Failure(err)
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return result.Failure(result.Usage("Gateway restart requires an invocation deadline"))
	}
	if err := ctx.Err(); err != nil {
		return result.Failure(err)
	}
	if required := runner.Require(workflow.GatewayRestart()); !required.OK {
		return required
	}
	out := observeRestart(runner)
	if !out.OK {
		return out
	}
	before := out.Data.(RestartObservation)
	evidence := RestartEvidence{Before: before, Last: before, Proof: "not_observed", Correlation: "selected_target"}
	baselineMeta := out.Meta
	request := execute.Request{Operation: workflow.RestartOperation, Query: url.Values{"confirm": {"true"}}, Yes: input.Yes, DryRun: input.DryRun}
	if err := ctx.Err(); err != nil {
		return restartFailure(out, evidence, err, false)
	}
	out = runner.Run(request)
	if out.Meta.Catalog == nil {
		out.Meta = baselineMeta
	}
	if input.DryRun {
		if !out.OK {
			return restartFailure(out, evidence, out.Error, false)
		}
		preview, ok := out.Data.(execute.Preview)
		if !ok {
			return restartFailure(out, evidence, restartProblem("response", "Gateway restart preview unavailable"), false)
		}
		evidence.Request = &preview
		out.Data = evidence
		return out
	}
	evidence.Acknowledged = out.OK
	if !out.OK && out.Outcome != "uncertain" && restartHTTPStatus(out) < 500 {
		return restartFailure(out, evidence, out.Error, false)
	}
	for {
		if err := ctx.Err(); err != nil {
			return restartFailure(out, evidence, err, true)
		}
		out = observeRestart(runner)
		evidence.Polls++
		if out.Meta.Catalog == nil {
			out.Meta = baselineMeta
		}
		if !out.OK {
			if !restartTransient(out) {
				return restartFailure(out, evidence, out.Error, true)
			}
		} else {
			evidence.Last = out.Data.(RestartObservation)
			if evidence.Last.NodeID != before.NodeID {
				return restartFailure(out, evidence, restartProblem("identity", "Gateway reported localId differs from the baseline; select a direct node URL before further action"), true)
			}
			evidence.Proof = "not_observed"
			if evidence.Last.ProcessID != before.ProcessID {
				evidence.Proof = "process_changed"
			} else if evidence.Last.Uptime < before.Uptime {
				evidence.Proof = "uptime_reset"
			}
			if evidence.Proof != "not_observed" && len(evidence.Last.Pending) == 0 {
				out.Data, out.Outcome, out.Meta.Verification = evidence, "completed", "restart_observed"
				out.Meta.Warnings = append(out.Meta.Warnings, "The reported process or uptime changed at the selected target with no pending restart tasks. Matching localId values do not prove node uniqueness; use a direct node URL. The API has no job ID or atomic node precondition; request causality and module health are not proven.")
				if !evidence.Acknowledged {
					out.Meta.Warnings = append(out.Meta.Warnings, "The restart request was not acknowledged. Only read-only verification continued; the request was not replayed.")
				}
				return out
			}
		}
		if err := waitPoll(ctx, input.Interval); err != nil {
			return restartFailure(out, evidence, err, true)
		}
	}
}

func restartHTTPStatus(out result.Result) int {
	if out.Error == nil {
		return out.Meta.HTTPStatus
	}
	raw, _ := json.Marshal(out.Error.Details)
	var details struct {
		HTTPStatus int `json:"httpStatus"`
	}
	_ = json.Unmarshal(raw, &details)
	return details.HTTPStatus
}

func restartTransient(out result.Result) bool {
	if out.Error == nil {
		return false
	}
	if out.Error.Kind == "transport" || out.Error.Kind == "timeout" {
		return true
	}
	if out.Error.Kind != "http" {
		return false
	}
	switch restartHTTPStatus(out) {
	case 429, 502, 503, 504:
		return true
	}
	return false
}

func restartProblem(kind, message string) *result.Problem {
	return &result.Problem{Kind: kind, Message: message, Code: 7}
}

func restartFailure(out result.Result, evidence RestartEvidence, err error, dispatched bool) result.Result {
	if err == nil {
		err = restartProblem("response", "Gateway restart verification failed")
	}
	out.OK, out.Outcome, out.Data, out.Error, out.Meta.Verification = false, "failed", evidence, result.FromError(err), "not_verified"
	if dispatched {
		out.Outcome = "uncertain"
		out.Meta.Warnings = append(out.Meta.Warnings, "The Gateway may still be restarting. Inspect its current state before deciding whether to retry; the CLI does not replay restart requests.")
	}
	return out
}
