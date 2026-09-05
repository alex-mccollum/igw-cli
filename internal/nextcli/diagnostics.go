package nextcli

import (
	"context"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/operations"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/spf13/cobra"
)

func (i *invocation) diagnosticsCommands() *cobra.Command {
	group := &cobra.Command{Use: "diagnostics", Short: "Inspect and collect Gateway diagnostics"}
	bundle := &cobra.Command{Use: "bundle", Short: "Work with the Gateway's latest diagnostic bundle"}
	status := &cobra.Command{Use: "status", Short: "Read whether the latest bundle is empty, generating, or ready", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return i.runWorkflow(cmd, false, func(_ context.Context, scope *execute.Scope) result.Result { return operations.BundleState(scope) })
	}}
	bundle.AddCommand(status)
	for _, generate := range []bool{false, true} {
		input := operations.BundleRequest{Generate: generate}
		name, short := "download", "Download the latest ready bundle and check its reported size"
		if generate {
			name, short = "collect", "Generate, wait for, and download the latest bundle"
		}
		cmd := &cobra.Command{Use: name, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if err := input.Validate(); err != nil {
				return err
			}
			return i.runWorkflow(cmd, input.Generate && !input.DryRun, func(ctx context.Context, scope *execute.Scope) result.Result {
				return operations.Bundle(ctx, scope, input)
			})
		}}
		f := cmd.Flags()
		f.StringVar(&input.Out, "out", "", "Destination ZIP file")
		_ = cmd.MarkFlagRequired("out")
		f.BoolVar(&input.Overwrite, "overwrite", false, "Replace an existing file after completion and size checks")
		f.Int64Var(&input.MaxBytes, "max-bytes", 1<<30, "Maximum download size in bytes (default 1 GiB)")
		if generate {
			f.BoolVar(&input.Yes, "yes", false, "Confirm diagnostics generation on the Gateway")
			f.BoolVar(&input.DryRun, "dry-run", false, "Inspect current status and preview generation without creating files or starting a job")
			f.DurationVar(&input.Interval, "interval", time.Second, "Status polling interval (100ms..1m); --timeout bounds the entire workflow")
		}
		bundle.AddCommand(cmd)
	}
	group.AddCommand(bundle)
	return group
}
