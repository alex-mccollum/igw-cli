package nextcli

import (
	"sort"

	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/spf13/cobra"
)

func (i *invocation) profileCommands() *cobra.Command {
	group := &cobra.Command{Use: "profile", Short: "Inspect configured profiles and effective targets"}
	group.AddCommand(&cobra.Command{Use: "list", Short: "List profile names without exposing credentials", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := i.app.ReadConfig()
			if err != nil {
				return result.Usage("could not read configuration")
			}
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			i.output = result.Success(map[string]any{"active": cfg.ActiveProfile, "profiles": names})
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
	return group
}
