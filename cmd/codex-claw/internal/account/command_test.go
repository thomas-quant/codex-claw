package account

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAccountCommand(t *testing.T) {
	cmd := NewAccountCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "account", cmd.Use)
	assert.Equal(t, "Manage Codex accounts", cmd.Short)
	assert.True(t, cmd.HasSubCommands())
	assert.NotNil(t, cmd.PersistentPreRunE)
	assert.NotNil(t, cmd.RunE)

	allowedCommands := []string{"add", "import", "list", "status", "remove", "enable", "disable"}
	subcommands := cmd.Commands()
	assert.Len(t, subcommands, len(allowedCommands))

	for _, subcmd := range subcommands {
		assert.True(t, slices.Contains(allowedCommands, subcmd.Name()), "unexpected subcommand %q", subcmd.Name())
		assert.NotNil(t, subcmd.RunE)
	}
}
