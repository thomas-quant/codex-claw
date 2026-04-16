package codexruntime

import (
	"testing"
	"time"
)

func TestBuildRuntimeStatusMergesBindingAndLiveState(t *testing.T) {
	t.Parallel()

	lastUserMessageAt := time.Date(2026, time.April, 12, 10, 0, 0, 0, time.UTC)
	lastCompactionAt := time.Date(2026, time.April, 12, 11, 0, 0, 0, time.UTC)

	status := BuildRuntimeStatus(RuntimeStatusInput{
		Binding: Binding{
			Key:               "telegram:chat-1:agent-coder",
			ThreadID:          "thr_123",
			Model:             "gpt-5.4",
			ThinkingMode:      "medium",
			FastEnabled:       true,
			LastUserMessageAt: lastUserMessageAt,
			Metadata: map[string]any{
				"last_compaction_at": lastCompactionAt.Format(time.RFC3339Nano),
				"recovery_mode":      "resume_after_restart",
			},
		},
		Client: ClientStatus{
			Started:          true,
			TurnActive:       true,
			KnownModels:      []string{"gpt-5.4", "gpt-5.4-mini"},
			LastCompactionAt: lastCompactionAt,
			Recovery: RecoveryStatus{
				RestartAttempted: true,
				ResumeAttempted:  true,
				FellBackToFresh:  false,
				Mode:             "resume_after_restart",
			},
		},
	})

	if status.BindingKey != "telegram:chat-1:agent-coder" {
		t.Fatalf("BindingKey = %q, want %q", status.BindingKey, "telegram:chat-1:agent-coder")
	}
	if status.ThreadID != "thr_123" || status.Model != "gpt-5.4" || status.ThinkingMode != "medium" || !status.FastEnabled {
		t.Fatalf("status = %#v, want bound thread/model/thinking/fast fields", status)
	}
	if !status.ClientStarted || !status.TurnActive {
		t.Fatalf("client state = %#v, want started active", status)
	}
	if status.LastUserMessageAt != lastUserMessageAt {
		t.Fatalf("LastUserMessageAt = %v, want %v", status.LastUserMessageAt, lastUserMessageAt)
	}
	if status.LastCompactionAt != lastCompactionAt {
		t.Fatalf("LastCompactionAt = %v, want %v", status.LastCompactionAt, lastCompactionAt)
	}
	if status.Recovery.Mode != "resume_after_restart" || !status.Recovery.RestartAttempted || !status.Recovery.ResumeAttempted {
		t.Fatalf("Recovery = %#v, want merged restart/resume state", status.Recovery)
	}
	if len(status.KnownModels) != 2 {
		t.Fatalf("KnownModels len = %d, want %d", len(status.KnownModels), 2)
	}
}

func TestBuildRuntimeStatusProjectsAccountMetadata(t *testing.T) {
	t.Parallel()

	status := BuildRuntimeStatus(RuntimeStatusInput{
		Binding: Binding{
			Key: "telegram:chat-1:agent-coder",
		},
		Client: ClientStatus{
			ActiveAccountAlias:   "alpha",
			AccountHealth:        "healthy",
			TelemetryFresh:       true,
			FiveHourRemainingPct: 88,
			WeeklyRemainingPct:   91,
			SwitchTrigger:        "soft_threshold_5h",
		},
	})

	if status.ActiveAccountAlias != "alpha" {
		t.Fatalf("ActiveAccountAlias = %q, want %q", status.ActiveAccountAlias, "alpha")
	}
	if status.AccountHealth != "healthy" {
		t.Fatalf("AccountHealth = %q, want %q", status.AccountHealth, "healthy")
	}
	if !status.TelemetryFresh {
		t.Fatal("TelemetryFresh = false, want true")
	}
	if status.FiveHourRemainingPct != 88 {
		t.Fatalf("FiveHourRemainingPct = %d, want %d", status.FiveHourRemainingPct, 88)
	}
	if status.WeeklyRemainingPct != 91 {
		t.Fatalf("WeeklyRemainingPct = %d, want %d", status.WeeklyRemainingPct, 91)
	}
	if status.SwitchTrigger != "soft_threshold_5h" {
		t.Fatalf("SwitchTrigger = %q, want %q", status.SwitchTrigger, "soft_threshold_5h")
	}
}
