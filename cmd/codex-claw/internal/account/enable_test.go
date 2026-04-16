package account

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewEnableCommand_EnablesAlias(t *testing.T) {
	t.Parallel()

	var gotAlias string
	cmd := newEnableCommand(func() (managerAPI, error) {
		return stubManager{
			enableFn: func(_ context.Context, alias string) error {
				gotAlias = alias
				return nil
			},
		}, nil
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"alpha"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAlias != "alpha" {
		t.Fatalf("alias = %q, want alpha", gotAlias)
	}
	if !strings.Contains(stdout.String(), "Enabled account") {
		t.Fatalf("output = %q, want enable confirmation", stdout.String())
	}
}
