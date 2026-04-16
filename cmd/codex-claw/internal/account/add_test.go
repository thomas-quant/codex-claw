package account

import (
	"bytes"
	"context"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

func TestNewAddCommand_PassesFlagsToManager(t *testing.T) {
	t.Parallel()

	var gotAlias string
	var gotOptions codexaccounts.AddOptions
	cmd := newAddCommand(func() (managerAPI, error) {
		return stubManager{
			addFn: func(_ context.Context, alias string, options codexaccounts.AddOptions) error {
				gotAlias = alias
				gotOptions = options
				return nil
			},
		}, nil
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"alpha", "--isolated", "--device-auth"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAlias != "alpha" {
		t.Fatalf("alias = %q, want alpha", gotAlias)
	}
	if !gotOptions.Isolated || !gotOptions.DeviceAuth {
		t.Fatalf("options = %+v, want isolated+device auth", gotOptions)
	}
	if got := stdout.String(); got == "" {
		t.Fatal("stdout empty, want success output")
	}
}
