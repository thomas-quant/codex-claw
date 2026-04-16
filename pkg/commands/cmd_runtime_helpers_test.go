package commands

import (
	"strings"
	"testing"
)

func TestFormatStatusSnapshot_IncludesAccountMetadata(t *testing.T) {
	t.Parallel()

	got := formatStatusSnapshot(StatusSnapshot{
		ThreadID:             "thr_123",
		Model:                "gpt-5.4",
		Provider:             "codex",
		ActiveAccountAlias:   "alpha",
		AccountHealth:        "healthy",
		TelemetryFresh:       true,
		FiveHourRemainingPct: 88,
		WeeklyRemainingPct:   91,
		SwitchTrigger:        "soft_threshold_5h",
	})

	for _, want := range []string{
		"Thread ID: thr_123",
		"Model: gpt-5.4 (Provider: codex)",
		"Active account: alpha",
		"Account health: healthy",
		"Telemetry fresh: yes",
		"5h remaining: 88%",
		"Weekly remaining: 91%",
		"Switch trigger: soft_threshold_5h",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatStatusSnapshot() = %q, want substring %q", got, want)
		}
	}
}
