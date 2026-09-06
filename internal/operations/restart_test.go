package operations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

type restartStep struct {
	op  string
	out result.Result
}

type restartRunner struct {
	t                              *testing.T
	steps                          []restartStep
	writes, previews, requirements int
	requirementResult              *result.Result
	after                          func(execute.Request)
}

func (r *restartRunner) Require(c catalog.Capability) result.Result {
	r.requirements++
	if c.ID != workflow.GatewayRestart().ID || len(c.RequiredOperations) != 4 {
		r.t.Fatal("restart did not require all routes")
	}
	if r.requirementResult != nil {
		return *r.requirementResult
	}
	return result.Success(nil)
}

func (r *restartRunner) Run(input execute.Request) result.Result {
	r.t.Helper()
	if len(r.steps) == 0 || r.steps[0].op != input.Operation {
		r.t.Fatalf("unexpected operation: %s", input.Operation)
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if input.Operation == workflow.RestartOperation {
		if input.Query.Encode() != "confirm=true" || len(input.Body) != 0 || input.Upload != nil {
			r.t.Fatal("restart wire contract changed")
		}
		if input.DryRun {
			r.previews++
		} else {
			if !input.Yes {
				r.t.Fatal("unconfirmed restart")
			}
			r.writes++
		}
	} else if input.MaxBodyBytes != 1<<20 {
		r.t.Fatal("unbounded restart observation")
	}
	if r.after != nil {
		r.after(input)
	}
	return step.out
}

func restartJSON(raw string) result.Result { return result.Success(json.RawMessage(raw)) }
func restartObservationSteps(node, overview, tasks, lastNode string) []restartStep {
	return []restartStep{
		{workflow.RedundancyOperation, restartJSON(node)},
		{workflow.OverviewOperation, restartJSON(overview)},
		{workflow.RestartTasksOperation, restartJSON(tasks)},
		{workflow.RedundancyOperation, restartJSON(lastNode)},
	}
}

const restartNodeJSON = `{"localId":"node-a","ignored":"private"}`
const restartBeforeJSON = `{"processId":100,"uptime":300,"hostname":"private"}`
const restartAfterJSON = `{"processId":101,"uptime":5}`
const restartNoTasksJSON = `{"pending":[]}`

func TestRestartProofAndSingleDispatch(t *testing.T) {
	for _, scenario := range []string{"changed pid", "uptime reset", "pending then clear", "transport then ready", "read timeout then ready", "http 503 then ready", "unacknowledged", "server error", "preview"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			r := &restartRunner{t: t, steps: restartObservationSteps(restartNodeJSON, restartBeforeJSON, `{"pending":["Module enabled"]}`, restartNodeJSON)}
			input := RestartRequest{Yes: true, Interval: 100 * time.Millisecond}
			ack := result.Success(nil)
			ack.Outcome = "accepted"
			if scenario == "unacknowledged" {
				ack = result.Failure(restartProblem("transport", "transfer failed"))
				ack.Outcome = "uncertain"
			}
			if scenario == "server error" {
				ack = result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 500}})
			}
			if scenario == "preview" {
				input.Yes, input.DryRun = false, true
				ack = result.Success(execute.Preview{Method: "POST", Path: "/data/api/v1/restart-tasks/restart", Mutating: true})
				ack.Outcome = "preview"
			}
			r.steps = append(r.steps, restartStep{workflow.RestartOperation, ack})
			if scenario == "pending then clear" {
				r.steps = append(r.steps, restartObservationSteps(restartNodeJSON, restartAfterJSON, `{"pending":["Module enabled"]}`, restartNodeJSON)...)
			}
			if strings.Contains(scenario, "then ready") {
				kind := "transport"
				if scenario == "read timeout then ready" {
					kind = "timeout"
				}
				failure := result.Failure(restartProblem(kind, "temporary read failure"))
				if scenario == "http 503 then ready" {
					failure.Error.Kind = "http"
					failure.Error.Details = map[string]int{"httpStatus": 503}
				}
				r.steps = append(r.steps, restartStep{workflow.RedundancyOperation, failure})
			}
			if scenario != "preview" {
				after := restartAfterJSON
				if scenario == "uptime reset" {
					after = `{"processId":100,"uptime":0.5}`
				}
				r.steps = append(r.steps, restartObservationSteps(restartNodeJSON, after, restartNoTasksJSON, restartNodeJSON)...)
			}
			out := Restart(ctx, r, input)
			if !out.OK || out.Error != nil || len(r.steps) != 0 || r.requirements != 1 {
				t.Fatalf("restart result: %+v, steps=%d", out, len(r.steps))
			}
			e := out.Data.(RestartEvidence)
			if e.Before.ProcessID != 100 || e.Before.NodeID != "node-a" || e.Correlation != "observed_node" {
				t.Fatal("baseline evidence lost")
			}
			if scenario == "preview" {
				if r.writes != 0 || r.previews != 1 || out.Outcome != "preview" || e.Request == nil || e.Polls != 0 || e.Acknowledged {
					t.Fatal("preview sent restart or claimed completion")
				}
			} else {
				wantProof := "process_changed"
				if scenario == "uptime reset" {
					wantProof = "uptime_reset"
				}
				if r.writes != 1 || out.Outcome != "completed" || out.Meta.Verification != "restart_observed" || e.Proof != wantProof || len(e.Last.Pending) != 0 {
					t.Fatalf("unverified restart: %+v", out)
				}
				if e.Acknowledged != (scenario != "unacknowledged" && scenario != "server error") || len(out.Meta.Warnings) == 0 {
					t.Fatal("acknowledgement or causality misrepresented")
				}
			}
			b, _ := json.Marshal(out)
			if strings.Contains(string(b), "private") {
				t.Fatal("unneeded Gateway fields escaped projection")
			}
		})
	}
}

func TestRestartStopsOnTerminalObservation(t *testing.T) {
	for _, scenario := range []string{"auth", "malformed", "node changed", "node changed within observation", "no process change", "tasks remain", "canceled before write", "post denied", "post redirect"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			r := &restartRunner{t: t, steps: restartObservationSteps(restartNodeJSON, restartBeforeJSON, restartNoTasksJSON, restartNodeJSON)}
			ack := result.Success(nil)
			if scenario == "post denied" {
				ack = result.Failure(&result.Problem{Kind: "auth", Code: 6, Details: map[string]int{"httpStatus": 403}})
			}
			if scenario == "post redirect" {
				ack = result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 307}})
			}
			if scenario == "canceled before write" {
				r.after = func(execute.Request) {
					if len(r.steps) == 0 {
						cancel()
					}
				}
			} else {
				r.steps = append(r.steps, restartStep{workflow.RestartOperation, ack})
			}
			switch scenario {
			case "auth":
				r.steps = append(r.steps, restartStep{workflow.RedundancyOperation, result.Failure(&result.Problem{Kind: "auth", Code: 6})})
			case "malformed":
				r.steps = append(r.steps, restartStep{workflow.RedundancyOperation, restartJSON(`{"localId":null}`)})
			case "node changed", "node changed within observation":
				first, last := `{"localId":"node-b"}`, `{"localId":"node-b"}`
				if scenario == "node changed within observation" {
					first = restartNodeJSON
				}
				r.steps = append(r.steps, restartObservationSteps(first, restartAfterJSON, restartNoTasksJSON, last)...)
			case "no process change":
				r.steps = append(r.steps, restartObservationSteps(restartNodeJSON, restartBeforeJSON, restartNoTasksJSON, restartNodeJSON)...)
			case "tasks remain":
				r.steps = append(r.steps, restartObservationSteps(restartNodeJSON, restartAfterJSON, `{"pending":["still pending"]}`, restartNodeJSON)...)
			}
			out := Restart(ctx, r, RestartRequest{Yes: true, Interval: 100 * time.Millisecond})
			if out.OK || out.Error == nil || r.writes > 1 || len(r.steps) != 0 {
				t.Fatalf("terminal result %+v", out)
			}
			wantOutcome, wantKind, wantCode := "uncertain", "timeout", 7
			switch scenario {
			case "auth":
				wantKind, wantCode = "auth", 6
			case "malformed":
				wantKind = "response"
			case "node changed", "node changed within observation":
				wantKind = "identity"
			case "canceled before write":
				wantOutcome, wantKind = "failed", "canceled"
				if r.writes != 0 {
					t.Fatal("cancellation still dispatched")
				}
			case "post denied":
				wantOutcome, wantKind, wantCode = "failed", "auth", 6
			case "post redirect":
				wantOutcome, wantKind = "failed", "http"
			}
			if out.Outcome != wantOutcome || out.Error.Kind != wantKind || out.Error.Code != wantCode || out.Meta.Verification != "not_verified" {
				t.Fatalf("incorrect terminal contract: %+v", out)
			}
		})
	}
}

func TestRestartBaselineRequiresValidPresentFields(t *testing.T) {
	for _, tc := range []struct{ field, raw string }{
		{"node", `{}`}, {"node", `null`}, {"node", `{"localId":""}`}, {"node", `{"localId":"a","localId":"b"}`}, {"node", `{"localId":"\ud800"}`},
		{"overview", `{}`}, {"overview", `{"processId":null,"uptime":1}`}, {"overview", `{"processId":0,"uptime":1}`}, {"overview", `{"processId":1.5,"uptime":1}`}, {"overview", `{"processId":1,"uptime":null}`}, {"overview", `{"processId":1,"uptime":-1}`}, {"overview", `{"processId":1,"uptime":-1e-999}`}, {"overview", `{"processId":1,"uptime":1e999}`}, {"overview", `{"processId":9223372036854775808,"uptime":1}`},
		{"tasks", `{}`}, {"tasks", `{"pending":null}`}, {"tasks", `{"pending":[null]}`}, {"tasks", `{"pending":[1]}`}, {"tasks", `{"pending":[],"pending":["task"]}`},
		{"last", `{"localId":"different"}`},
	} {
		t.Run(tc.field+tc.raw, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			steps := restartObservationSteps(restartNodeJSON, restartBeforeJSON, restartNoTasksJSON, restartNodeJSON)
			index := map[string]int{"node": 0, "overview": 1, "tasks": 2, "last": 3}[tc.field]
			steps[index].out = restartJSON(tc.raw)
			r := &restartRunner{t: t, steps: steps[:index+1]}
			out := Restart(ctx, r, RestartRequest{Yes: true, Interval: time.Second})
			if out.OK || out.Outcome != "failed" || r.writes != 0 || len(r.steps) != 0 || out.Error.Code != 7 {
				t.Fatalf("invalid baseline accepted: %+v", out)
			}
		})
	}
}

func TestRestartPreflightRefusesWithoutIO(t *testing.T) {
	for _, tc := range []string{"confirmation", "both flags", "short interval", "long interval", "deadline", "capability", "canceled"} {
		t.Run(tc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			input := RestartRequest{Yes: true, Interval: time.Second}
			r := &restartRunner{t: t}
			switch tc {
			case "confirmation":
				input.Yes = false
			case "both flags":
				input.DryRun = true
			case "short interval":
				input.Interval = time.Millisecond
			case "long interval":
				input.Interval = 2 * time.Minute
			case "deadline":
				ctx = context.Background()
			case "canceled":
				cancel()
			case "capability":
				failure := result.Failure(&result.Problem{Kind: "capability", Code: 2})
				r.requirementResult = &failure
			}
			out := Restart(ctx, r, input)
			if out.OK || out.Error == nil || r.writes != 0 || r.previews != 0 {
				t.Fatal("invalid preflight proceeded")
			}
		})
	}
}
