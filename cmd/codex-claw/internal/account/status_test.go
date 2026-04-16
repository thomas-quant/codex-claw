package account

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

func TestNewStatusCommand_RendersSummary(t *testing.T) {
	t.Parallel()

	cmd := newStatusCommand(func() (managerAPI, error) {
		return stubManager{
			statusFn: func(context.Context) (codexaccounts.StatusSummary, error) {
				return codexaccounts.StatusSummary{
					TotalAccounts: 2,
					EnabledCount:  1,
					ActiveAlias:   "alpha",
				}, nil
			},
		}, nil
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Active account: alpha") {
		t.Fatalf("output = %q, want active account line", got)
	}
	if !strings.Contains(got, "Configured accounts: 2") {
		t.Fatalf("output = %q, want configured count", got)
	}
}
