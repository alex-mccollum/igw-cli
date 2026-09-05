package nextcli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func inputProblem(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return result.Usage(err.Error())
}

func (i *invocation) runWorkflow(cmd *cobra.Command, write bool, run func(context.Context, *execute.Scope) result.Result) error {
	if i.offline {
		return result.Usage("this workflow requires current Gateway state; use offline catalog inspection for its contract")
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
	engine := execute.Engine{Catalog: service, HTTP: i.app.HTTP}
	scope, err := engine.Open(ctx, target, token, catalog.Policy{ForWrite: write, AllowStale: i.allowStale, Pin: i.pin})
	if err != nil {
		return sourceProblem(err)
	}
	defer scope.Close()
	i.output = run(ctx, scope)
	return nil
}
