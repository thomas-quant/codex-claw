package account

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

func newAddCommand(factory managerFactory) *cobra.Command {
	var isolated bool
	var deviceAuth bool

	cmd := &cobra.Command{
		Use:   "add <alias>",
		Short: "Add a Codex account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := factory()
			if err != nil {
				return err
			}
			if err := manager.Add(cmd.Context(), args[0], codexaccounts.AddOptions{
				Isolated:   isolated,
				DeviceAuth: deviceAuth,
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Added account %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&isolated, "isolated", false, "Run login in an isolated Codex home")
	cmd.Flags().BoolVar(&deviceAuth, "device-auth", false, "Use device authorization instead of browser login")

	return cmd
}
