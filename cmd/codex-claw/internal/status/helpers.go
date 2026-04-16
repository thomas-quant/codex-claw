package status

import (
	"fmt"
	"os"

	"github.com/thomas-quant/codex-claw/cmd/codex-claw/internal"
	"github.com/thomas-quant/codex-claw/cmd/codex-claw/internal/cliui"
	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
	"github.com/thomas-quant/codex-claw/pkg/config"
)

type accountSummary struct {
	total  int
	active string
}

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
	}

	if configOK {
		report.Model = cfg.Agents.Defaults.GetModelName()
		if report.Model == "" {
			report.Model = cfg.Runtime.Codex.DefaultModel
		}
		if report.Model == "" {
			report.Model = "not configured"
		}

		val := func(enabled bool, extra ...string) string {
			if enabled {
				if len(extra) > 0 && extra[0] != "" {
					return "✓ " + extra[0]
				}
				return "✓"
			}
			return "not set"
		}

		report.Providers = []cliui.ProviderRow{
			{Name: "Codex app-server", Val: val(true)},
			{
				Name: "DeepSeek fallback",
				Val:  val(cfg.Runtime.Fallback.DeepSeek.Enabled, cfg.Runtime.Fallback.DeepSeek.Model),
			},
		}
		report.Providers = append(report.Providers, accountProviderRows(loadAccountSummary(internal.GetCodexClawHome()))...)
	}

	cliui.PrintStatus(report)
}

func loadAccountSummary(home string) accountSummary {
	manager := codexaccounts.NewManager(codexaccounts.ResolveLayout(home), codexaccounts.ManagerOptions{})
	summary, err := manager.Status(nil)
	if err != nil {
		return accountSummary{}
	}
	return accountSummary{
		total:  summary.TotalAccounts,
		active: summary.ActiveAlias,
	}
}

func accountProviderRows(summary accountSummary) []cliui.ProviderRow {
	rows := []cliui.ProviderRow{
		{Name: "Codex accounts", Val: "not set"},
		{Name: "Active account", Val: "not set"},
	}
	if summary.total > 0 {
		rows[0].Val = fmt.Sprintf("✓ %d configured", summary.total)
	}
	if summary.active != "" {
		rows[1].Val = "✓ " + summary.active
	}
	return rows
}
