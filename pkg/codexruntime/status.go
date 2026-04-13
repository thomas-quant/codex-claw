package codexruntime

import "time"

type RecoveryStatus struct {
	RestartAttempted bool   `json:"restart_attempted"`
	ResumeAttempted  bool   `json:"resume_attempted"`
	FellBackToFresh  bool   `json:"fell_back_to_fresh"`
	Mode             string `json:"mode,omitempty"`
}

type ClientStatus struct {
	Started          bool           `json:"started"`
	TurnActive       bool           `json:"turn_active"`
	KnownModels      []string       `json:"known_models,omitempty"`
	LastCompactionAt time.Time      `json:"last_compaction_at,omitempty"`
	Recovery         RecoveryStatus `json:"recovery"`
}

type RuntimeStatusInput struct {
	Binding Binding
	Client  ClientStatus
}

type RuntimeStatusSnapshot struct {
	BindingKey        string         `json:"binding_key"`
	ThreadID          string         `json:"thread_id,omitempty"`
	Model             string         `json:"model,omitempty"`
	ThinkingMode      string         `json:"thinking_mode,omitempty"`
	FastEnabled       bool           `json:"fast_enabled"`
	LastUserMessageAt time.Time      `json:"last_user_message_at,omitempty"`
	LastCompactionAt  time.Time      `json:"last_compaction_at,omitempty"`
	ClientStarted     bool           `json:"client_started"`
	TurnActive        bool           `json:"turn_active"`
	KnownModels       []string       `json:"known_models,omitempty"`
	Recovery          RecoveryStatus `json:"recovery"`
}

func BuildRuntimeStatus(input RuntimeStatusInput) RuntimeStatusSnapshot {
	snapshot := RuntimeStatusSnapshot{
		BindingKey:        input.Binding.Key,
		ThreadID:          input.Binding.ThreadID,
		Model:             input.Binding.Model,
		ThinkingMode:      input.Binding.ThinkingMode,
		FastEnabled:       input.Binding.FastEnabled,
		LastUserMessageAt: input.Binding.LastUserMessageAt,
		ClientStarted:     input.Client.Started,
		TurnActive:        input.Client.TurnActive,
		KnownModels:       append([]string(nil), input.Client.KnownModels...),
		Recovery:          input.Client.Recovery,
	}

	if !input.Client.LastCompactionAt.IsZero() {
		snapshot.LastCompactionAt = input.Client.LastCompactionAt
	} else if raw, ok := input.Binding.Metadata["last_compaction_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			snapshot.LastCompactionAt = parsed
		}
	}
	if snapshot.Recovery.Mode == "" {
		if mode, ok := input.Binding.Metadata["recovery_mode"].(string); ok {
			snapshot.Recovery.Mode = mode
		}
	}
	if !snapshot.Recovery.RestartAttempted {
		if attempted, ok := input.Binding.Metadata["restart_attempted"].(bool); ok {
			snapshot.Recovery.RestartAttempted = attempted
		}
	}
	if !snapshot.Recovery.ResumeAttempted {
		if attempted, ok := input.Binding.Metadata["resume_attempted"].(bool); ok {
			snapshot.Recovery.ResumeAttempted = attempted
		}
	}
	if !snapshot.Recovery.FellBackToFresh {
		if fresh, ok := input.Binding.Metadata["fell_back_to_fresh"].(bool); ok {
			snapshot.Recovery.FellBackToFresh = fresh
		}
	}

	return snapshot
}
