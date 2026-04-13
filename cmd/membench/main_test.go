package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootCommand(t *testing.T) {
	cmd := newRootCommand()

	require.NotNil(t, cmd)

	assert.Equal(t, "membench", cmd.Use)
	assert.Equal(t, "Memory benchmark tool for codex-claw", cmd.Short)
	assert.Len(t, cmd.Commands(), 4)
}
