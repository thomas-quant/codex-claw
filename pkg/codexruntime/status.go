package codexruntime

import "time"

type RecoveryStatus struct {
	RestartAttempted bool   `json:"restart_attempted"`
	ResumeAttempted  bool   `json:"resume_attempted"`
	FellBackToFresh  bool   `json:"fell_back_to_fresh"`
	Mode             string `json:"mode,omitempty"`
}

type ClientStatus struct {
	Started              bool           `json:"started"`
	TurnActive           bool           `json:"turn_active"`
	KnownModels          []string       `json:"known_models,omitempty"`
	LastCompactionAt     time.Time      `json:"last_compaction_at,omitempty"`
	Recovery             RecoveryStatus `json:"recovery"`
	ActiveAccountAlias   string         `json:"active_account_alias,omitempty"`
	AccountHealth        string         `json:"account_health,omitempty"`
	TelemetryFresh       bool           `json:"telemetry_fresh"`
	FiveHourRemainingPct int            `json:"five_hour_remaining_pct,omitempty"`
	WeeklyRemainingPct   int            `json:"weekly_remaining_pct,omitempty"`
	SwitchTrigger        string         `json:"switch_trigger,omitempty"`
}

type RuntimeStatusInput struct {
	Binding Binding
	Client  ClientStatus
}

type RuntimeStatusSnapshot struct {
	BindingKey           string         `json:"binding_key"`
	ThreadID             string         `json:"thread_id,omitempty"`
	Model                string         `json:"model,omitempty"`
	ThinkingMode         string         `json:"thinking_mode,omitempty"`
	FastEnabled          bool           `json:"fast_enabled"`
	LastUserMessageAt    time.Time      `json:"last_user_message_at,omitempty"`
	LastCompactionAt     time.Time      `json:"last_compaction_at,omitempty"`
	ForceFreshThread     bool           `json:"force_fresh_thread"`
	ClientStarted        bool           `json:"client_started"`
	TurnActive           bool           `json:"turn_active"`
	KnownModels          []string       `json:"known_models,omitempty"`
	Recovery             RecoveryStatus `json:"recovery"`
	ActiveAccountAlias   string         `json:"active_account_alias,omitempty"`
	AccountHealth        string         `json:"account_health,omitempty"`
	TelemetryFresh       bool           `json:"telemetry_fresh"`
	FiveHourRemainingPct int            `json:"five_hour_remaining_pct,omitempty"`
	WeeklyRemainingPct   int            `json:"weekly_remaining_pct,omitempty"`
	SwitchTrigger        string         `json:"switch_trigger,omitempty"`
}

func BuildRuntimeStatus(input RuntimeStatusInput) RuntimeStatusSnapshot {
	snapshot := RuntimeStatusSnapshot{
		BindingKey:           input.Binding.Key,
		ThreadID:             input.Binding.ThreadID,
		Model:                input.Binding.Model,
		ThinkingMode:         input.Binding.ThinkingMode,
		FastEnabled:          input.Binding.FastEnabled,
		LastUserMessageAt:    input.Binding.LastUserMessageAt,
		ClientStarted:        input.Client.Started,
		TurnActive:           input.Client.TurnActive,
		KnownModels:          append([]string(nil), input.Client.KnownModels...),
		Recovery:             input.Client.Recovery,
		ActiveAccountAlias:   input.Client.ActiveAccountAlias,
		AccountHealth:        input.Client.AccountHealth,
		TelemetryFresh:       input.Client.TelemetryFresh,
		FiveHourRemainingPct: input.Client.FiveHourRemainingPct,
		WeeklyRemainingPct:   input.Client.WeeklyRemainingPct,
		SwitchTrigger:        input.Client.SwitchTrigger,
	}

	if !input.Client.LastCompactionAt.IsZero() {
		snapshot.LastCompactionAt = input.Client.LastCompactionAt
	} else if raw, ok := input.Binding.Metadata[bindingMetadataLastCompactionAt].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			snapshot.LastCompactionAt = parsed
		}
	}
	if snapshot.Recovery.Mode == "" {
		if mode, ok := input.Binding.Metadata[bindingMetadataRecoveryMode].(string); ok {
			snapshot.Recovery.Mode = mode
		}
	}
	if !snapshot.Recovery.RestartAttempted {
		if attempted, ok := input.Binding.Metadata[bindingMetadataRestartAttempted].(bool); ok {
			snapshot.Recovery.RestartAttempted = attempted
		}
	}
	if !snapshot.Recovery.ResumeAttempted {
		if attempted, ok := input.Binding.Metadata[bindingMetadataResumeAttempted].(bool); ok {
			snapshot.Recovery.ResumeAttempted = attempted
		}
	}
	if !snapshot.Recovery.FellBackToFresh {
		if fresh, ok := input.Binding.Metadata[bindingMetadataFellBackToFresh].(bool); ok {
			snapshot.Recovery.FellBackToFresh = fresh
		}
	}
	if fresh, ok := input.Binding.Metadata[bindingMetadataForceFreshThread].(bool); ok {
		snapshot.ForceFreshThread = fresh
	}
	if snapshot.ActiveAccountAlias == "" {
		if alias, ok := input.Binding.Metadata["active_account_alias"].(string); ok {
			snapshot.ActiveAccountAlias = alias
		}
	}
	if snapshot.AccountHealth == "" {
		if health, ok := input.Binding.Metadata["account_health"].(string); ok {
			snapshot.AccountHealth = health
		}
	}
	if !snapshot.TelemetryFresh {
		if fresh, ok := input.Binding.Metadata["telemetry_fresh"].(bool); ok {
			snapshot.TelemetryFresh = fresh
		}
	}
	if snapshot.FiveHourRemainingPct == 0 {
		if pct, ok := input.Binding.Metadata["five_hour_remaining_pct"].(int); ok {
			snapshot.FiveHourRemainingPct = pct
		}
	}
	if snapshot.WeeklyRemainingPct == 0 {
		if pct, ok := input.Binding.Metadata["weekly_remaining_pct"].(int); ok {
			snapshot.WeeklyRemainingPct = pct
		}
	}
	if snapshot.SwitchTrigger == "" {
		if trigger, ok := input.Binding.Metadata["switch_trigger"].(string); ok {
			snapshot.SwitchTrigger = trigger
		}
	}

	return snapshot
}
