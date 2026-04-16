package codexaccounts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_WriteSnapshotStateHealthAndSwitchAudit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewStore(ResolveLayout(root))

	if err := store.WriteSnapshot("alpha", []byte(`{"last_refresh":"2026-04-14T09:00:00Z"}`)); err != nil {
		t.Fatalf("WriteSnapshot() error = %v", err)
	}
	if err := store.SaveState(State{
		Version:     1,
		ActiveAlias: "alpha",
		Accounts: map[string]AccountState{
			"alpha": {Alias: "alpha", Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := store.SaveHealth(map[string]HealthSnapshot{
		"alpha": {
			Alias:                "alpha",
			Status:               "healthy",
			FiveHourRemainingPct: 88,
			WeeklyRemainingPct:   91,
			ObservedAt:           time.Date(2026, time.April, 14, 11, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("SaveHealth() error = %v", err)
	}
	if err := store.AppendSwitchEvent(SwitchEvent{
		OccurredAt:       time.Date(2026, time.April, 14, 11, 5, 0, 0, time.UTC),
		SourceAlias:      "alpha",
		TargetAlias:      "beta",
		Trigger:          "soft_threshold_5h",
		RouteReason:      "best_5h_headroom",
		ResumeMode:       "same_thread_resume",
		TelemetryFresh:   true,
		AppServerRestart: false,
	}); err != nil {
		t.Fatalf("AppendSwitchEvent() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "codex-accounts", "accounts", "alpha.json")); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "codex-accounts", "state.json")); err != nil {
		t.Fatalf("state missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "codex-accounts", "health.json")); err != nil {
		t.Fatalf("health missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "codex-accounts", "switches.jsonl")); err != nil {
		t.Fatalf("switch audit missing: %v", err)
	}
}

func TestStore_WriteSnapshot_RejectsUnsafeAlias(t *testing.T) {
	t.Parallel()

	store := NewStore(ResolveLayout(t.TempDir()))
	if err := store.WriteSnapshot("../beta", []byte(`{}`)); err == nil {
		t.Fatal("WriteSnapshot() error = nil, want alias validation failure")
	}
}
