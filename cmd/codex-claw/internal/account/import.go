package account

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

func newImportCommand(factory managerFactory) *cobra.Command {
	var authFile string

	cmd := &cobra.Command{
		Use:   "import <alias>",
		Short: "Import an existing Codex auth snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := factory()
			if err != nil {
				return err
			}
			if err := manager.Import(cmd.Context(), args[0], codexaccounts.ImportOptions{
				AuthFile: authFile,
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported account %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&authFile, "auth-file", "", "Path to an existing Codex auth.json to import")
	_ = cmd.MarkFlagRequired("auth-file")

	return cmd
}
