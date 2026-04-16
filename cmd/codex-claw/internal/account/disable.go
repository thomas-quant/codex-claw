package account

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDisableCommand(factory managerFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <alias>",
		Short: "Disable a Codex account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := factory()
			if err != nil {
				return err
			}
			if err := manager.Disable(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Disabled account %s\n", args[0])
			return nil
		},
	}
}
