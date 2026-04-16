package cliui

import (
	"strings"
	"testing"
)

func TestProviderTablePanel_RendersAccountRows(t *testing.T) {
	t.Parallel()

	got := providerTablePanel(StatusReport{
		ConfigOK: true,
		Providers: []ProviderRow{
			{Name: "Codex accounts", Val: "✓ 3 configured"},
			{Name: "Active account", Val: "✓ alpha"},
		},
	}, 60)

	if !strings.Contains(got, "Codex accounts") {
		t.Fatalf("panel missing Codex accounts row: %q", got)
	}
	if !strings.Contains(got, "Active account") {
		t.Fatalf("panel missing Active account row: %q", got)
	}
}
