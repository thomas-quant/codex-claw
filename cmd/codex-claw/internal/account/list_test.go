package account

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

func TestNewListCommand_RendersAccounts(t *testing.T) {
	t.Parallel()

	cmd := newListCommand(func() (managerAPI, error) {
		return stubManager{
			listFn: func(context.Context) ([]codexaccounts.AccountSummary, error) {
				return []codexaccounts.AccountSummary{
					{Alias: "alpha", Enabled: true, Active: true},
					{Alias: "beta", Enabled: false},
				}, nil
			},
		}, nil
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "* alpha") {
		t.Fatalf("output missing active account: %q", got)
	}
	if !strings.Contains(got, "beta") || !strings.Contains(got, "disabled") {
		t.Fatalf("output missing disabled account: %q", got)
	}
}
