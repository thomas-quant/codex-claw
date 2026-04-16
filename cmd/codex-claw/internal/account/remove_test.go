package account

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewRemoveCommand_RemovesAlias(t *testing.T) {
	t.Parallel()

	var gotAlias string
	cmd := newRemoveCommand(func() (managerAPI, error) {
		return stubManager{
			removeFn: func(_ context.Context, alias string) error {
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
	if !strings.Contains(stdout.String(), "Removed account") {
		t.Fatalf("output = %q, want remove confirmation", stdout.String())
	}
}
