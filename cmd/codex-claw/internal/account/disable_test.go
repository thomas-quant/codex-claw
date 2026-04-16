package account

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewDisableCommand_DisablesAlias(t *testing.T) {
	t.Parallel()

	var gotAlias string
	cmd := newDisableCommand(func() (managerAPI, error) {
		return stubManager{
			disableFn: func(_ context.Context, alias string) error {
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
	if !strings.Contains(stdout.String(), "Disabled account") {
		t.Fatalf("output = %q, want disable confirmation", stdout.String())
	}
}
