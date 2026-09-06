package cli

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/spf13/cobra"
)

func (i *invocation) profileCommands() *cobra.Command {
	group := &cobra.Command{Use: "profile", Short: "Configure profiles, inspect targets, and migrate local settings"}
	group.AddCommand(&cobra.Command{Use: "list", Short: "List profile names without exposing credentials", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var state config.State
			var cfg config.File
			var err error
			if i.app.ConfigStore != nil {
				state, err = i.app.ConfigStore.Read()
				if err != nil {
					return result.Usage(err.Error())
				}
				cfg = state.Config
			} else {
				cfg, err = i.app.ReadConfig()
			}
			if err != nil {
				return result.Usage("could not read configuration")
			}
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			i.output = result.Success(map[string]any{"active": cfg.ActiveProfile, "profiles": names, "source": state.Source, "revision": state.Revision})
			return nil
		}})
	group.AddCommand(&cobra.Command{Use: "show", Short: "Show the effective target and whether a token is configured", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, token, err := i.runtime()
			if err != nil {
				return err
			}
			i.output = result.Success(map[string]any{"target": target, "tokenConfigured": token != ""})
			return nil
		}})
	group.AddCommand(i.profileSet(), i.profileUse(), i.profileRemove(), i.profileMigration(false), i.profileMigration(true))
	return group
}

func profileChangeFlags(cmd *cobra.Command, r *config.Change) {
	f := cmd.Flags()
	f.BoolVar(&r.Yes, "yes", false, "Apply this local configuration change")
	f.BoolVar(&r.DryRun, "dry-run", false, "Preview without writing configuration or contacting a Gateway")
	f.StringVar(&r.IfRevision, "if-revision", "", "Require the current v1 revision from profile list or a preview")
}

func (i *invocation) profileWrite(cmd *cobra.Command, r config.Change) error {
	if i.app.ConfigStore == nil {
		return result.Usage("profile writes require a configuration store")
	}
	if cmd.Flags().Changed("profile") || cmd.Flags().Changed("gateway-url") {
		return result.Usage("profile edits use a positional name and --url; runtime --profile and --gateway-url do not select stored fields")
	}
	ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
	if err != nil {
		return err
	}
	defer cancel()
	change, err := i.app.ConfigStore.Change(ctx, r)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return result.Usage(err.Error())
	}
	i.output = result.Success(change)
	if !change.Applied {
		i.output.Outcome = "preview"
	}
	return nil
}

func (i *invocation) profileSet() *cobra.Command {
	r := config.Change{Action: "set"}
	var url string
	var tokenStdin, clearToken bool
	cmd := &cobra.Command{Use: "set NAME", Short: "Create or update a profile with explicit stored values", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if r.Yes == r.DryRun {
			return result.Usage("choose --dry-run to preview or --yes to change local configuration")
		}
		if tokenStdin && clearToken {
			return result.Usage("choose --token-stdin or --clear-token")
		}
		if cmd.Flags().Changed("profile") || cmd.Flags().Changed("gateway-url") {
			return result.Usage("profile set uses NAME and --url, not runtime target overrides")
		}
		r.Name = args[0]
		if cmd.Flags().Changed("url") {
			r.GatewayURL = &url
		}
		if tokenStdin {
			ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
			if err != nil {
				return err
			}
			defer cancel()
			raw, err := readProfileToken(ctx, i.app.In)
			if err != nil {
				return err
			}
			token := strings.TrimSpace(string(raw))
			if token == "" {
				return result.Usage("stdin token is empty; use --clear-token to remove it")
			}
			r.Token = &token
			// Preserve the same total deadline through the subsequent local edit.
			cmd.SetContext(ctx)
		}
		if clearToken {
			token := ""
			r.Token = &token
		}
		return i.profileWrite(cmd, r)
	}}
	f := cmd.Flags()
	f.StringVar(&url, "url", "", "Store this Gateway URL; environment and runtime overrides are not saved")
	f.BoolVar(&tokenStdin, "token-stdin", false, "Read a token from stdin (maximum 16 KiB); never display it")
	f.BoolVar(&clearToken, "clear-token", false, "Remove the stored token")
	f.BoolVar(&r.Activate, "use", false, "Make this profile active after setting it")
	profileChangeFlags(cmd, &r)
	cmd.ValidArgsFunction = i.completeProfileArgument
	return cmd
}

// A blocked stdin must not prevent Ctrl-C or the invocation deadline from
// returning. The CLI process exits after Run; embedded callers must eventually
// unblock/close their supplied reader so the single bounded read can finish.
func readProfileToken(ctx context.Context, in io.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type readResult struct {
		raw []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() { raw, err := io.ReadAll(io.LimitReader(in, (16<<10)+1)); done <- readResult{raw, err} }()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case got := <-done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if got.err != nil || len(got.raw) > 16<<10 {
			return nil, result.Usage("could not read token within 16 KiB")
		}
		return got.raw, nil
	}
}

func (i *invocation) profileUse() *cobra.Command {
	r := config.Change{Action: "use"}
	cmd := &cobra.Command{Use: "use [NAME]", Short: "Select the active profile or preserved defaults", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			r.Name = args[0]
		}
		return i.profileWrite(cmd, r)
	}}
	cmd.Flags().BoolVar(&r.UseDefaults, "default", false, "Clear the active profile and use default settings plus environment/flags")
	profileChangeFlags(cmd, &r)
	cmd.ValidArgsFunction = i.completeProfileArgument
	return cmd
}

func (i *invocation) profileRemove() *cobra.Command {
	r := config.Change{Action: "remove"}
	cmd := &cobra.Command{Use: "remove NAME", Short: "Remove an inactive profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r.Name = args[0]
		return i.profileWrite(cmd, r)
	}}
	profileChangeFlags(cmd, &r)
	cmd.ValidArgsFunction = i.completeProfileArgument
	return cmd
}

func (i *invocation) completeProfileArgument(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return i.completeProfiles(cmd, args, prefix)
}

func (i *invocation) completeProfiles(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	cfg, err := i.app.ReadConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	names := []string{}
	for name := range cfg.Profiles {
		if strings.HasPrefix(name, prefix) && !strings.ContainsAny(name, "\t\r\n") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func (i *invocation) profileMigration(rollback bool) *cobra.Command {
	r := config.Change{Action: "migrate"}
	short := "Copy current legacy settings into v1 without modifying the legacy file"
	if rollback {
		r.Action = "rollback"
		short = "Archive v1 and return to the unchanged legacy configuration"
	}
	cmd := &cobra.Command{Use: r.Action, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return i.profileWrite(cmd, r) }}
	profileChangeFlags(cmd, &r)
	return cmd
}
