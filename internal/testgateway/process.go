package testgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
)

// ProcessObservation independently binds an API-reported PID to a Java process
// inside the exact owned container. It never reads environment or command-line
// arguments. StartTicks is Linux /proc/PID/stat field 22, without unit conversion.
type ProcessObservation struct {
	ContainerID         string    `json:"containerId"`
	ContainerStartedAt  time.Time `json:"containerStartedAt"`
	ContainerRestarts   int       `json:"containerRestarts"`
	ProcessID           int64     `json:"processId"`
	ExecutableName      string    `json:"executableName"`
	StartTicks          uint64    `json:"startTicks"`
	ContainmentVerified bool      `json:"containmentVerified"`
	ObservedAt          time.Time `json:"observedAt"`
}

func (s *Session) ObserveJavaProcess(ctx context.Context, pid int64) (ProcessObservation, error) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	var out ProcessObservation
	if !s.created || s.closed || !containerID.MatchString(s.ID) || s.owner == "" || pid <= 0 {
		return out, errors.New("process observation requires a live owned session and positive PID")
	}
	raw, err := s.command(ctx, nil, "inspect", "--format", `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "`+ownerLabel+`")}},"running":{{json .State.Running}},"startedAt":{{json .State.StartedAt}},"restarts":{{json .RestartCount}}}`, s.ID)
	if err != nil {
		return out, err
	}
	var identity struct {
		ID, Owner string
		Running   bool
		StartedAt time.Time
		Restarts  int
	}
	if json.Unmarshal(raw, &identity) != nil || identity.ID != s.ID || identity.Owner != s.owner || !identity.Running || identity.StartedAt.IsZero() || identity.Restarts != 0 {
		return out, errors.New("process observation refused unowned, stopped, or restarted container")
	}
	if err := s.verifyConfiguration(ctx); err != nil {
		return out, err
	}
	if err := s.verifyKernelLimits(ctx); err != nil {
		return out, err
	}
	proc := "/proc/" + strconv.FormatInt(pid, 10)
	raw, err = s.command(ctx, nil, "exec", s.ID, "readlink", proc+"/exe")
	if err != nil {
		return out, err
	}
	executable := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(executable, "/") || path.Base(executable) != "java" {
		return out, fmt.Errorf("API process %d runs executable %q; Java verification unavailable", pid, path.Base(executable))
	}
	raw, err = s.command(ctx, nil, "exec", s.ID, "cat", proc+"/comm", proc+"/stat")
	if err != nil {
		return out, err
	}
	ticks, err := javaStartTicks(raw, pid)
	if err != nil {
		return out, err
	}
	return ProcessObservation{ContainerID: s.ID, ContainerStartedAt: identity.StartedAt, ContainerRestarts: identity.Restarts, ProcessID: pid, ExecutableName: "java", StartTicks: ticks, ContainmentVerified: true, ObservedAt: time.Now().UTC()}, nil
}

func javaStartTicks(raw []byte, pid int64) (uint64, error) {
	comm, stat, ok := strings.Cut(string(raw), "\n")
	// Linux comm is a thread name, which the JVM can change independently of
	// its executable. Match the two proc observations without assuming "java".
	prefix := strconv.FormatInt(pid, 10) + " (" + comm + ") "
	if !ok || comm == "" || len(comm) > 64 || !strings.HasPrefix(stat, prefix) {
		return 0, errors.New("API process ID and process status did not agree")
	}
	fields := strings.Fields(strings.TrimPrefix(stat, prefix))
	// fields[0] is state (field 3); starttime is field 22.
	if len(fields) < 20 || (fields[0] != "R" && fields[0] != "S" && fields[0] != "D") {
		return 0, errors.New("Java process status is incomplete or not running")
	}
	ticks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || ticks == 0 {
		return 0, errors.New("Java process start time is unavailable")
	}
	return ticks, nil
}
