package testgateway_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/operations"
)

type inputRestartObservation struct {
	NodeSHA256   string  `json:"nodeSha256"`
	ProcessID    int64   `json:"processId"`
	Uptime       float64 `json:"uptime"`
	PendingCount int     `json:"pendingCount"`
}

type inputRestartEvidence struct {
	Before            inputRestartObservation `json:"before"`
	Last              inputRestartObservation `json:"last"`
	Polls             int                     `json:"polls"`
	Acknowledged      bool                    `json:"acknowledged"`
	Proof             string                  `json:"proof"`
	Correlation       string                  `json:"correlation"`
	Request           *execute.Preview        `json:"request,omitempty"`
	ConfirmedRequests int                     `json:"confirmedRequests"`
}

func restartEvidence(raw json.RawMessage) *inputRestartEvidence {
	var e operations.RestartEvidence
	if json.Unmarshal(raw, &e) != nil || e.Before.NodeID == "" || e.Last.NodeID == "" {
		return nil
	}
	project := func(o operations.RestartObservation) inputRestartObservation {
		return inputRestartObservation{NodeSHA256: inputDigest([]byte(o.NodeID)), ProcessID: o.ProcessID, Uptime: o.Uptime, PendingCount: len(o.Pending)}
	}
	return &inputRestartEvidence{Before: project(e.Before), Last: project(e.Last), Polls: e.Polls, Acknowledged: e.Acknowledged, Proof: e.Proof, Correlation: e.Correlation, Request: e.Request}
}

// Opt-in only: this restarts the Java Gateway process inside one guarded,
// freshly commissioned owned container. It never restarts the container,
// Docker engine, Docker Desktop, or WSL.
func TestLiveRestart(t *testing.T) {
	s := beginObservedSuite(t, "IGW_RESTART_EVIDENCE_DIR", "gateway-restart", "restart.json")
	s.refused(s.run("confirmation", nil, "gateway", "restart"))
	s.refused(s.run("conflicting-flags", nil, "gateway", "restart", "--yes", "--dry-run"))
	s.refused(s.run("offline-preview", nil, "gateway", "restart", "--dry-run", "--offline"))
	preview := s.run("preview", nil, "gateway", "restart", "--dry-run")
	s.require(preview, 4)
	p := s.last().Restart
	if preview.Outcome != "preview" || p == nil || p.Request == nil || !p.Request.Mutating || p.Request.Method != "POST" || p.Request.Path != "/data/api/v1/restart-tasks/restart" || p.ConfirmedRequests != 0 || p.Polls != 0 {
		t.Fatal("restart preview did not retain baseline and prepared request")
	}
	before, err := s.session.ObserveJavaProcess(s.ctx, p.Before.ProcessID)
	if err != nil {
		t.Fatal(err)
	}
	s.receipt.Checks[len(s.receipt.Checks)-1].Process = &before
	if p.Before.ProcessID != p.Last.ProcessID || p.Before.NodeSHA256 != p.Last.NodeSHA256 {
		t.Fatal("preview changed its observed identity")
	}
	for _, wire := range s.last().Wire {
		if wire.Method != "GET" {
			t.Fatal("preview sent a mutation")
		}
	}

	restarted := s.run("restart", nil, "gateway", "restart", "--yes", "--interval", "500ms")
	r := s.last().Restart
	if !restarted.OK || restarted.Outcome != "completed" || restarted.Meta.Verification != "restart_observed" || r == nil || (r.Proof != "process_changed" && r.Proof != "uptime_reset") || r.Polls < 1 || r.ConfirmedRequests != 1 || r.Before.ProcessID != before.ReportedProcessID || r.Before.NodeSHA256 != p.Before.NodeSHA256 || r.Last.NodeSHA256 != r.Before.NodeSHA256 || r.Last.PendingCount != 0 {
		t.Fatalf("restart was not verified (kind=%v)", restarted.Error)
	}
	writes := 0
	for _, wire := range s.last().Wire {
		if wire.Method == "POST" {
			writes++
		} else if wire.Method != "GET" {
			t.Fatal("restart workflow sent an unexpected method")
		}
	}
	if writes != 1 || s.last().CatalogRequests != 1 {
		t.Fatal("restart did not use one fresh catalog and one mutation")
	}
	after, err := s.session.ObserveJavaProcess(s.ctx, r.Last.ProcessID)
	if err != nil {
		t.Fatal(err)
	}
	s.receipt.Checks[len(s.receipt.Checks)-1].Process = &after
	if after.ContainerID != before.ContainerID || !after.ContainerStartedAt.Equal(before.ContainerStartedAt) || after.ContainerRestarts != 0 || after.ProcessID == before.ProcessID || after.StartTicks <= before.StartTicks || !after.ContainmentVerified || !before.ContainmentVerified {
		t.Fatal("restart was not independently a later Java process in the same contained Gateway")
	}
	s.require(s.run("tasks-after", nil, "gateway", "restart-tasks"), 1)
	s.require(s.run("doctor-after", nil, "gateway", "doctor"), 1)
	s.require(s.run("preview-after", nil, "gateway", "restart", "--dry-run"), 4)
	last := s.last().Restart
	if last == nil || last.Before.ProcessID != after.ReportedProcessID || last.Before.NodeSHA256 != r.Before.NodeSHA256 || last.Before.PendingCount != 0 || last.ConfirmedRequests != 0 {
		t.Fatal("independent post-restart baseline disagreed")
	}
	s.completed = true
}

func TestRestartEvidenceOmitsInstanceValues(t *testing.T) {
	before := operations.RestartObservation{NodeID: "private-node", ProcessID: 42, Uptime: 200, Pending: []string{"private-task"}}
	e := operations.RestartEvidence{Before: before, Last: before, Polls: 2, Proof: "process_changed", Correlation: "observed_node"}
	b, _ := json.Marshal(e)
	projected := restartEvidence(b)
	if projected == nil || projected.Before.NodeSHA256 != inputDigest([]byte("private-node")) || projected.Before.PendingCount != 1 || projected.Polls != 2 {
		t.Fatal("restart evidence projection lost required checks")
	}
	b, _ = json.Marshal(projected)
	if strings.Contains(string(b), "private-node") || strings.Contains(string(b), "private-task") {
		t.Fatal("restart receipt exposed instance values")
	}
}
