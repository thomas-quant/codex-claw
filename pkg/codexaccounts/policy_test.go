package codexaccounts

import (
	"testing"
	"time"
)

func TestChooseTarget_PrefersWeeklyHealthyThenBestFiveHour(t *testing.T) {
	t.Parallel()

	target, reason, ok := ChooseTarget("alpha", []HealthSnapshot{
		{Alias: "beta", Status: "healthy", FiveHourRemainingPct: 72, WeeklyRemainingPct: 44},
		{Alias: "gamma", Status: "healthy", FiveHourRemainingPct: 80, WeeklyRemainingPct: 15},
		{Alias: "delta", Status: "healthy", FiveHourRemainingPct: 65, WeeklyRemainingPct: 61},
	})
	if !ok || target != "beta" || reason != "best_5h_headroom" {
		t.Fatalf("ChooseTarget() = (%q, %q, %v), want beta / best_5h_headroom / true", target, reason, ok)
	}
}

func TestShouldSoftSwitch_RequiresFreshTelemetry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 14, 13, 0, 0, 0, time.UTC)
	if ShouldSoftSwitch(now, HealthSnapshot{
		Alias:                "alpha",
		FiveHourRemainingPct: 8,
		ObservedAt:           now.Add(-20 * time.Minute),
	}) {
		t.Fatal("ShouldSoftSwitch() = true, want false for stale telemetry")
	}
}
