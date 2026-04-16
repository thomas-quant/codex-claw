package onboard

import (
	"embed"

	"github.com/spf13/cobra"
)

//go:generate cp -r ../../../../workspace .
//go:embed workspace
var embeddedFiles embed.FS

func NewOnboardCommand() *cobra.Command {
	var encrypt bool
	var surface string
	var importAuthFile string

	cmd := &cobra.Command{
		Use:     "onboard",
		Aliases: []string{"o"},
		Short:   "Initialize codex-claw configuration and workspace",
		// Run without subcommands → original onboard flow
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				onboard(onboardOptions{
					Encrypt:        encrypt,
					Surface:        surface,
					ImportAuthFile: importAuthFile,
				})
			} else {
				_ = cmd.Help()
			}
		},
	}

	cmd.Flags().BoolVar(&encrypt, "enc", false,
		"Enable credential encryption (generates SSH key and prompts for passphrase)")
	cmd.Flags().StringVar(&surface, "surface", "", "Initial chat surface: telegram or discord")
	cmd.Flags().StringVar(&importAuthFile, "import-auth-file", "",
		"Import an existing Codex auth.json into the managed live Codex home")

	return cmd
}
