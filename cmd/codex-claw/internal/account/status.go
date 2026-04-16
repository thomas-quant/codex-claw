package account

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCommand(factory managerFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Codex account status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := factory()
			if err != nil {
				return err
			}
			summary, err := manager.Status(cmd.Context())
			if err != nil {
				return err
			}
			active := "none"
			if summary.ActiveAlias != "" {
				active = summary.ActiveAlias
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Configured accounts: %d\n", summary.TotalAccounts)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Enabled accounts: %d\n", summary.EnabledCount)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Active account: %s\n", active)
			return nil
		},
	}
}
