package migrate

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate from xxxclaw(openclaw, etc.) to picoclaw",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("migration runtime is not available in this build")
		},
	}

	return cmd
}
