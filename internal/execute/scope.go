package execute

import (
	"context"
	"sync"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

// Scope owns one target, credential, catalog, and cancellation budget for a
// workflow. Steps run serially; Close cancels in-flight work and releases the
// catalog. A write scope verifies freshness before the first state read.
type Scope struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	engine   Engine
	target   catalog.Target
	token    string
	policy   catalog.Policy
	snapshot *catalog.Snapshot
}

func (e Engine) Open(ctx context.Context, target catalog.Target, token string, policy catalog.Policy) (*Scope, error) {
	ctx, cancel := context.WithCancel(ctx)
	snapshot, err := e.Catalog.Acquire(ctx, target, token, policy)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		cancel()
		snapshot.Close()
		return nil, catalogProblem(err)
	}
	return &Scope{ctx: ctx, cancel: cancel, engine: e, target: target, token: token, policy: policy, snapshot: snapshot}, nil
}

func (s *Scope) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Close()
	s.snapshot, s.token = nil, ""
}

func (s *Scope) Run(input Request) result.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot == nil {
		return result.Failure(result.Usage("workflow scope is closed"))
	}
	if err := s.ctx.Err(); err != nil {
		return result.Failure(err)
	}
	if input.Operation == "" || input.Method != "" || input.Path != "" {
		return result.Failure(result.Usage("workflow steps require a catalog operation"))
	}
	if input.Offline || input.AllowStale || input.Pin != "" {
		return result.Failure(result.Usage("catalog policies belong to the workflow invocation"))
	}
	op, err := s.snapshot.Catalog.Resolve(input.Operation)
	if err != nil {
		return result.Failure(result.Usage(err.Error()))
	}
	if mutating(op.Method) && !input.DryRun && !s.policy.ForWrite {
		return result.Failure(result.Usage("workflow was opened for read-only access"))
	}
	input.Offline, input.AllowStale, input.Pin = s.policy.Offline, s.policy.AllowStale, s.policy.Pin
	prepared, err := s.engine.prepare(s.ctx, s.target, s.token, input, s.snapshot)
	if err != nil {
		return result.Failure(err)
	}
	return s.engine.Execute(s.ctx, prepared, s.token)
}
