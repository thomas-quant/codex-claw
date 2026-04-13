package version

import (
	"github.com/spf13/cobra"

	"github.com/sipeed/codex-claw/cmd/codex-claw/internal"
	"github.com/sipeed/codex-claw/cmd/codex-claw/internal/cliui"
	"github.com/sipeed/codex-claw/pkg/config"
)

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Show version information",
		Run: func(_ *cobra.Command, _ []string) {
			printVersion()
		},
	}

	return cmd
}

func printVersion() {
	build, goVer := config.FormatBuildInfo()
	cliui.PrintVersion(internal.Logo, "codex-claw "+config.FormatVersion(), build, goVer)
}
