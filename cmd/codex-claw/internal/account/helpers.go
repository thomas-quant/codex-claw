package account

import (
	"context"

	"github.com/thomas-quant/codex-claw/cmd/codex-claw/internal"
	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

type managerAPI interface {
	Add(context.Context, string, codexaccounts.AddOptions) error
	Import(context.Context, string, codexaccounts.ImportOptions) error
	List(context.Context) ([]codexaccounts.AccountSummary, error)
	Status(context.Context) (codexaccounts.StatusSummary, error)
	Remove(context.Context, string) error
	Enable(context.Context, string) error
	Disable(context.Context, string) error
}

type managerFactory func() (managerAPI, error)

func defaultManagerFactory() (managerAPI, error) {
	layout := codexaccounts.ResolveLayout(internal.GetCodexClawHome())
	return codexaccounts.NewManager(layout, codexaccounts.ManagerOptions{}), nil
}
