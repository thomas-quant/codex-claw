package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStatusCommand(t *testing.T) {
	cmd := NewStatusCommand()

	require.NotNil(t, cmd)

	assert.Equal(t, "status", cmd.Use)

	assert.Len(t, cmd.Aliases, 1)
	assert.True(t, cmd.HasAlias("s"))

	assert.Equal(t, "Show codex-claw status", cmd.Short)

	assert.False(t, cmd.HasSubCommands())

	assert.NotNil(t, cmd.Run)
	assert.Nil(t, cmd.RunE)

	assert.Nil(t, cmd.PersistentPreRun)
	assert.Nil(t, cmd.PersistentPostRun)
}

func TestAccountProviderRows(t *testing.T) {
	t.Parallel()

	rows := accountProviderRows(accountSummary{
		total:  2,
		active: "alpha",
	})

	require.Len(t, rows, 2)
	assert.Equal(t, "Codex accounts", rows[0].Name)
	assert.Equal(t, "✓ 2 configured", rows[0].Val)
	assert.Equal(t, "Active account", rows[1].Name)
	assert.Equal(t, "✓ alpha", rows[1].Val)
}
