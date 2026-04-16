package account

import (
	"github.com/spf13/cobra"
)

func NewAccountCommand() *cobra.Command {
	factory := defaultManagerFactory

	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manage Codex accounts",
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			_, err := factory()
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newAddCommand(factory),
		newImportCommand(factory),
		newListCommand(factory),
		newStatusCommand(factory),
		newRemoveCommand(factory),
		newEnableCommand(factory),
		newDisableCommand(factory),
	)

	return cmd
}
