package nextcli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/operations"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func (i *invocation) gatewayCommands() *cobra.Command {
	group := &cobra.Command{Use: "gateway", Short: "Inspect Gateway status and verify an explicit restart"}
	group.AddCommand(&cobra.Command{Use: "doctor", Short: "Read Gateway information without sending mutations", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/gateway-info"})
		}})
	group.AddCommand(&cobra.Command{Use: "restart-tasks", Short: "Read tasks awaiting a Gateway restart", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return i.runWorkflow(cmd, false, func(_ context.Context, scope *execute.Scope) result.Result { return operations.RestartTasks(scope) })
		}})
	var input operations.RestartRequest
	restart := &cobra.Command{Use: "restart", Short: "Restart the Gateway and verify reported process or uptime changes", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := input.Validate(); err != nil {
				return err
			}
			return i.runWorkflow(cmd, !input.DryRun, func(ctx context.Context, scope *execute.Scope) result.Result {
				return operations.Restart(ctx, scope, input)
			})
		}}
	f := restart.Flags()
	f.BoolVar(&input.Yes, "yes", false, "Confirm a full Gateway restart, including interruption of running services")
	f.BoolVar(&input.DryRun, "dry-run", false, "Read node, process, and pending tasks; prepare the request without restarting")
	f.DurationVar(&input.Interval, "interval", time.Second, "Read-only polling interval (100ms..1m); --timeout bounds discovery, restart, and verification")
	group.AddCommand(restart)
	return group
}
