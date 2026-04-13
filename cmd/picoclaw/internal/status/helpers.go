package status

import (
	"fmt"
	"os"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/cmd/picoclaw/internal/cliui"
	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
)

func statusCmd() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	configPath := internal.GetConfigPath()
	build, _ := config.FormatBuildInfo()

	_, configStatErr := os.Stat(configPath)
	configOK := configStatErr == nil

	workspace := cfg.WorkspacePath()
	_, wsErr := os.Stat(workspace)
	wsOK := wsErr == nil

	report := cliui.StatusReport{
		Logo:          internal.Logo,
		Version:       config.FormatVersion(),
		Build:         build,
		ConfigPath:    configPath,
		ConfigOK:      configOK,
		WorkspacePath: workspace,
		WorkspaceOK:   wsOK,
		Model:         cfg.Runtime.Codex.DefaultModel,
	}

	if configOK {
		report.Providers = []cliui.ProviderRow{
			{Name: "Runtime model", Val: "✓"},
		}

		store, _ := auth.LoadStore()
		if store != nil && len(store.Credentials) > 0 {
			for provider, cred := range store.Credentials {
				st := "authenticated"
				if cred.IsExpired() {
					st = "expired"
				} else if cred.NeedsRefresh() {
					st = "needs refresh"
				}
				report.OAuthLines = append(report.OAuthLines,
					fmt.Sprintf("%s (%s): %s", provider, cred.AuthMethod, st))
			}
		}
	}

	cliui.PrintStatus(report)
}
