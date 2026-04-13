package agent

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewAgentCommand() *cobra.Command {
	var (
		model      string
		debug      bool
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Interact with the agent directly",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("agent runtime is not available in this build")
		},
	}

	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	cmd.Flags().StringVarP(&model, "model", "", "", "Model to use")

	return cmd
}
