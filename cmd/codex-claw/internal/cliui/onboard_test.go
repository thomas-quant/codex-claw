package cliui

import (
	"strings"
	"testing"
)

func TestBuildOnboardingSteps_IncludesSurfaceAndAuthImport(t *testing.T) {
	t.Parallel()

	got := buildOnboardingSteps(false, "/tmp/config.json", "discord", true)

	for _, want := range []string{
		"Review your runtime settings in",
		"/tmp/config.json",
		"Finish your discord setup in .security.yml",
		"Managed live Codex auth imported",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("buildOnboardingSteps() missing %q in %q", want, got)
		}
	}
}

func TestChatStep_UsesSelectedSurface(t *testing.T) {
	t.Parallel()

	got := chatStep(true, "telegram")
	if !strings.Contains(got, "Open your telegram chat") {
		t.Fatalf("chatStep() = %q, want telegram-specific next step", got)
	}
	if !strings.Contains(got, "4. ") {
		t.Fatalf("chatStep() = %q, want encryption/import adjusted numbering", got)
	}
}
