package cron

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisableSubcommand(t *testing.T) {
	fn := func() string { return "" }
	cmd := newDisableCommand(fn)

	require.NotNil(t, cmd)

	assert.Equal(t, "disable", cmd.Use)
	assert.Equal(t, "Disable a job", cmd.Short)
	assert.Equal(t, "codex-claw cron disable 1", cmd.Example)

	assert.True(t, cmd.HasExample())
}
