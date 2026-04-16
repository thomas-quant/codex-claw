package account

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCommand(factory managerFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Codex accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := factory()
			if err != nil {
				return err
			}
			accounts, err := manager.List(cmd.Context())
			if err != nil {
				return err
			}
			if len(accounts) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No Codex accounts configured.")
				return nil
			}
			for _, account := range accounts {
				prefix := "  "
				if account.Active {
					prefix = "* "
				}
				status := "enabled"
				if !account.Enabled {
					status = "disabled"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s%s (%s)\n", prefix, account.Alias, status)
			}
			return nil
		},
	}
}
