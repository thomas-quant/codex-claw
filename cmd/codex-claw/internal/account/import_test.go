package account

import (
	"bytes"
	"context"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

func TestNewImportCommand_PassesFlagsToManager(t *testing.T) {
	t.Parallel()

	var gotAlias string
	var gotOptions codexaccounts.ImportOptions
	cmd := newImportCommand(func() (managerAPI, error) {
		return stubManager{
			importFn: func(_ context.Context, alias string, options codexaccounts.ImportOptions) error {
				gotAlias = alias
				gotOptions = options
				return nil
			},
		}, nil
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"alpha", "--auth-file", "/tmp/codex-auth.json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAlias != "alpha" {
		t.Fatalf("alias = %q, want alpha", gotAlias)
	}
	if gotOptions.AuthFile != "/tmp/codex-auth.json" {
		t.Fatalf("options = %+v, want auth file to be passed through", gotOptions)
	}
	if got := stdout.String(); got == "" {
		t.Fatal("stdout empty, want success output")
	}
}
