package codexruntime

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestRunner_ResumeFallsBackToStart(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_old",
		Model:    "gpt-5.2",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		resumeErr:       errors.New("resume failed"),
		startThreadID:   "thr_new",
		assistantChunks: []string{"Hello", " world"},
	}
	var chunks []string

	runner := NewRunner(client, store)
	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "hi",
		Recovery: RecoveryRequest{
			AllowResume: true,
		},
		OnChunk: func(chunk string) {
			chunks = append(chunks, chunk)
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.Content != "Hello world" {
		t.Fatalf("RunTextTurn() content = %q, want %q", got.Content, "Hello world")
	}
	if got.ThreadID != "thr_new" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_new")
	}
	if !client.started {
		t.Fatal("expected runner to fall back to thread/start")
	}
	if client.resumedWith != "thr_old" {
		t.Fatalf("ResumeThread() used thread_id = %q, want %q", client.resumedWith, "thr_old")
	}
	if client.runThreadID != "thr_new" {
		t.Fatalf("RunTextTurn() used thread_id = %q, want %q", client.runThreadID, "thr_new")
	}
	if !slices.Equal(chunks, []string{"Hello", " world"}) {
		t.Fatalf("streamed chunks = %v, want %v", chunks, []string{"Hello", " world"})
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to remain present")
	}
	if binding.ThreadID != "thr_new" {
		t.Fatalf("Load() thread_id = %q, want %q", binding.ThreadID, "thr_new")
	}
	if binding.Model != "gpt-5.4" {
		t.Fatalf("Load() model = %q, want %q", binding.Model, "gpt-5.4")
	}
}

func TestRunner_ResumedThreadKeepsStoredModel(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_existing",
		Model:    "gpt-5.2",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		assistantChunks: []string{"kept"},
	}

	runner := NewRunner(client, store)
	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "hi",
		Recovery: RecoveryRequest{
			AllowResume: true,
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.ThreadID != "thr_existing" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_existing")
	}
	if client.resumedWith != "thr_existing" {
		t.Fatalf("ResumeThread() used thread_id = %q, want %q", client.resumedWith, "thr_existing")
	}
	if client.started {
		t.Fatal("expected runner to reuse saved thread without thread/start")
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to remain present")
	}
	if binding.ThreadID != "thr_existing" {
		t.Fatalf("Load() thread_id = %q, want %q", binding.ThreadID, "thr_existing")
	}
	if binding.Model != "gpt-5.2" {
		t.Fatalf("Load() model = %q, want %q", binding.Model, "gpt-5.2")
	}
}

func TestRunner_ZeroValueRecoveryStartsFreshThread(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_existing",
		Model:    "gpt-5.2",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		startThreadID:   "thr_new",
		assistantChunks: []string{"fresh"},
	}

	runner := NewRunner(client, store)
	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.ThreadID != "thr_new" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_new")
	}
	if client.resumeCalls != 0 {
		t.Fatalf("ResumeThread() calls = %d, want %d", client.resumeCalls, 0)
	}
	if !client.started {
		t.Fatal("expected runner to start a fresh thread")
	}
}

func TestRunner_FreshThreadSavesRequestedModel(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())

	client := &fakeRunnerClient{
		startThreadID:   "thr_new",
		assistantChunks: []string{"kept"},
	}

	runner := NewRunner(client, store)
	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.ThreadID != "thr_new" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_new")
	}
	if !client.started {
		t.Fatal("expected runner to start a fresh thread")
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to be present")
	}
	if binding.ThreadID != "thr_new" {
		t.Fatalf("Load() thread_id = %q, want %q", binding.ThreadID, "thr_new")
	}
	if binding.Model != "gpt-5.4" {
		t.Fatalf("Load() model = %q, want %q", binding.Model, "gpt-5.4")
	}
}

func TestRunner_RestartsThenRetriesResumeBeforeStartingFresh(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_existing",
		Model:    "gpt-5.2",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		resumeErrs:      []error{errors.New("resume failed once"), nil},
		assistantChunks: []string{"resumed"},
	}

	runner := NewRunner(client, store)
	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "hi",
		Recovery: RecoveryRequest{
			AllowServerRestart: true,
			AllowResume:        true,
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.ThreadID != "thr_existing" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_existing")
	}
	if client.restartCalls != 1 {
		t.Fatalf("Restart() calls = %d, want %d", client.restartCalls, 1)
	}
	if client.resumeCalls != 2 {
		t.Fatalf("ResumeThread() calls = %d, want %d", client.resumeCalls, 2)
	}
	if client.started {
		t.Fatal("expected runner to avoid thread/start after successful retry resume")
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected binding to remain present")
	}
	if binding.Metadata["recovery_mode"] != "resume_after_restart" {
		t.Fatalf("Load() recovery_mode = %#v, want %q", binding.Metadata["recovery_mode"], "resume_after_restart")
	}
}

func TestRunner_ForceFreshThreadSkipsResumeAndStartsNewThread(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_old",
		Model:    "gpt-5.4",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		startThreadID:   "thr_new",
		assistantChunks: []string{"fresh"},
	}
	runner := NewRunner(client, store)

	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "bootstrap payload",
		Recovery: RecoveryRequest{
			AllowResume:        true,
			AllowServerRestart: true,
		},
		Control: ControlRequest{
			ForceFreshThread: true,
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.ThreadID != "thr_new" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_new")
	}
	if client.resumeCalls != 0 {
		t.Fatalf("ResumeThread() calls = %d, want %d", client.resumeCalls, 0)
	}
	if !client.started {
		t.Fatal("expected runner to start a fresh thread")
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to remain present")
	}
	if binding.Metadata["force_fresh_thread"] != nil {
		t.Fatalf("Load() force_fresh_thread = %#v, want cleared after consuming force-fresh control", binding.Metadata["force_fresh_thread"])
	}
	if binding.Metadata["recovery_mode"] != recoveryModeFresh {
		t.Fatalf("Load() recovery_mode = %#v, want %q", binding.Metadata["recovery_mode"], recoveryModeFresh)
	}
	if fellBackToFresh, _ := binding.Metadata["fell_back_to_fresh"].(bool); fellBackToFresh {
		t.Fatalf("Load() fell_back_to_fresh = %#v, want false for intentional fresh start", binding.Metadata["fell_back_to_fresh"])
	}
}

func TestRunner_ReadRateLimits_UsesClientRPC(t *testing.T) {
	t.Parallel()

	client := &fakeRunnerClient{
		rateLimits: []RateLimitSnapshot{
			{ID: "codex", PlanType: "plus"},
		},
	}
	runner := NewRunner(client, NewBindingStore(t.TempDir()))

	limits, err := runner.ReadRateLimits(context.Background())
	if err != nil {
		t.Fatalf("ReadRateLimits() error = %v", err)
	}
	if len(limits) != 1 || limits[0].ID != "codex" {
		t.Fatalf("ReadRateLimits() = %#v, want codex limit", limits)
	}
	if client.startCalls != 1 {
		t.Fatalf("Start() calls = %d, want %d", client.startCalls, 1)
	}
}

func TestRunner_StartsClientBeforeFreshThread(t *testing.T) {
	t.Parallel()

	client := &fakeRunnerClient{
		startThreadID:   "thr_new",
		assistantChunks: []string{"OK"},
	}
	runner := NewRunner(client, NewBindingStore(t.TempDir()))

	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "cli:diag",
		Model:      "gpt-5.4-mini",
		InputText:  "Reply with OK and nothing else.",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if got.Content != "OK" {
		t.Fatalf("RunTextTurn() content = %q, want %q", got.Content, "OK")
	}
	if client.startCalls != 1 {
		t.Fatalf("Start() calls = %d, want %d", client.startCalls, 1)
	}
	if !client.started {
		t.Fatal("expected StartThread() after Start()")
	}
}

func TestRunner_ResumeFailureFallsBackToFreshWithSeededInput(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_old",
		Model:    "gpt-5.4",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		resumeErrs:      []error{errors.New("resume failed"), errors.New("resume failed again")},
		startThreadID:   "thr_new",
		assistantChunks: []string{"fresh"},
	}
	runner := NewRunner(client, store)

	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "USER: old\nASSISTANT: old reply\nUSER: current",
		Recovery: RecoveryRequest{
			AllowResume:        true,
			AllowServerRestart: true,
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if got.ThreadID != "thr_new" {
		t.Fatalf("RunTextTurn() thread_id = %q, want %q", got.ThreadID, "thr_new")
	}
	if client.resumeCalls != 2 {
		t.Fatalf("ResumeThread() calls = %d, want %d", client.resumeCalls, 2)
	}
	if client.restartCalls != 1 {
		t.Fatalf("Restart() calls = %d, want %d", client.restartCalls, 1)
	}
	if client.runInput != "USER: old\nASSISTANT: old reply\nUSER: current" {
		t.Fatalf("RunTextTurn() input = %q, want seeded fresh input", client.runInput)
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to remain present")
	}
	if binding.Metadata["recovery_mode"] != recoveryModeFresh {
		t.Fatalf("Load() recovery_mode = %#v, want %q", binding.Metadata["recovery_mode"], recoveryModeFresh)
	}
	if restartAttempted, _ := binding.Metadata["restart_attempted"].(bool); !restartAttempted {
		t.Fatalf("Load() restart_attempted = %#v, want true", binding.Metadata["restart_attempted"])
	}
	if resumeAttempted, _ := binding.Metadata["resume_attempted"].(bool); !resumeAttempted {
		t.Fatalf("Load() resume_attempted = %#v, want true", binding.Metadata["resume_attempted"])
	}
	if fellBackToFresh, _ := binding.Metadata["fell_back_to_fresh"].(bool); !fellBackToFresh {
		t.Fatalf("Load() fell_back_to_fresh = %#v, want true", binding.Metadata["fell_back_to_fresh"])
	}
}

func TestRunner_ResumeFallbackPreservesStoredFastEnabledWhenRequestOmitsIt(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:         "telegram:chat-1:coder",
		ThreadID:    "thr_old",
		Model:       "gpt-5.4",
		FastEnabled: true,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		resumeErrs:      []error{errors.New("resume failed"), errors.New("resume failed again")},
		startThreadID:   "thr_new",
		assistantChunks: []string{"fresh"},
	}
	runner := NewRunner(client, store)

	_, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "USER: old\nASSISTANT: old reply\nUSER: current",
		Recovery: RecoveryRequest{
			AllowResume:        true,
			AllowServerRestart: true,
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to remain present")
	}
	if !binding.FastEnabled {
		t.Fatal("Load() fast_enabled = false, want true")
	}
}

func TestRunner_ResumeFallbackClearsStoredFastEnabledWhenRequestExplicitlyDisablesIt(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:         "telegram:chat-1:coder",
		ThreadID:    "thr_old",
		Model:       "gpt-5.4",
		FastEnabled: true,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		resumeErrs:      []error{errors.New("resume failed"), errors.New("resume failed again")},
		startThreadID:   "thr_new",
		assistantChunks: []string{"fresh"},
	}
	runner := NewRunner(client, store)

	_, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "USER: old\nASSISTANT: old reply\nUSER: current",
		Recovery: RecoveryRequest{
			AllowResume:        true,
			AllowServerRestart: true,
		},
		Control: ControlRequest{
			FastEnabled:    false,
			FastEnabledSet: true,
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved binding to remain present")
	}
	if binding.FastEnabled {
		t.Fatal("Load() fast_enabled = true, want false")
	}
}

func TestRunner_CompactThreadStartsNativeCompactionAndPersistsMarker(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:      "telegram:chat-1:coder",
		ThreadID: "thr_existing",
		Model:    "gpt-5.2",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{}
	runner := NewRunner(client, store)

	if err := runner.CompactThread(context.Background(), "telegram:chat-1:coder"); err != nil {
		t.Fatalf("CompactThread() error = %v", err)
	}
	if client.compactedThreadID != "thr_existing" {
		t.Fatalf("StartNativeCompaction() thread_id = %q, want %q", client.compactedThreadID, "thr_existing")
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected binding to remain present")
	}
	value, ok := binding.Metadata["last_compaction_at"].(string)
	if !ok || value == "" {
		t.Fatalf("Load() last_compaction_at = %#v, want RFC3339 timestamp", binding.Metadata["last_compaction_at"])
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		t.Fatalf("Parse(last_compaction_at) error = %v", err)
	}
}

func TestRunner_SetRuntimeControlsPersistBinding(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:          "telegram:chat-1:coder",
		ThreadID:     "thr_existing",
		Model:        "gpt-5.4",
		ThinkingMode: "medium",
		FastEnabled:  false,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	runner := NewRunner(&fakeRunnerClient{}, store)

	oldModel, err := runner.SetModel(context.Background(), "telegram:chat-1:coder", "gpt-5.4-mini")
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if oldModel != "gpt-5.4" {
		t.Fatalf("SetModel() old = %q, want %q", oldModel, "gpt-5.4")
	}

	oldThinking, err := runner.SetThinkingMode(context.Background(), "telegram:chat-1:coder", "high")
	if err != nil {
		t.Fatalf("SetThinkingMode() error = %v", err)
	}
	if oldThinking != "medium" {
		t.Fatalf("SetThinkingMode() old = %q, want %q", oldThinking, "medium")
	}

	fastEnabled, err := runner.ToggleFast(context.Background(), "telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("ToggleFast() error = %v", err)
	}
	if !fastEnabled {
		t.Fatal("ToggleFast() = false, want true")
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected binding to remain present")
	}
	if binding.Model != "gpt-5.4-mini" {
		t.Fatalf("Load() model = %q, want %q", binding.Model, "gpt-5.4-mini")
	}
	if binding.ThinkingMode != "high" {
		t.Fatalf("Load() thinking_mode = %q, want %q", binding.ThinkingMode, "high")
	}
	if !binding.FastEnabled {
		t.Fatal("Load() fast_enabled = false, want true")
	}
}

func TestRunner_ResetThreadClearsOnlyThreadID(t *testing.T) {
	t.Parallel()

	lastUserMessageAt := time.Date(2026, time.April, 12, 14, 30, 0, 0, time.UTC)
	lastCompactionAt := time.Date(2026, time.April, 12, 14, 31, 0, 0, time.UTC)

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:               "telegram:chat-1:coder",
		ThreadID:          "thr_existing",
		Model:             "gpt-5.4-mini",
		ThinkingMode:      "high",
		FastEnabled:       true,
		LastUserMessageAt: lastUserMessageAt,
		Metadata: map[string]any{
			"recovery_mode":      "resumed",
			"restart_attempted":  true,
			"resume_attempted":   true,
			"fell_back_to_fresh": true,
			"force_fresh_thread": true,
			"last_compaction_at": lastCompactionAt.Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	runner := NewRunner(&fakeRunnerClient{}, store)
	if err := runner.ResetThread(context.Background(), "telegram:chat-1:coder"); err != nil {
		t.Fatalf("ResetThread() error = %v", err)
	}

	binding, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("expected binding to remain present")
	}
	if binding.ThreadID != "" {
		t.Fatalf("Load() thread_id = %q, want empty", binding.ThreadID)
	}
	if binding.Model != "gpt-5.4-mini" {
		t.Fatalf("Load() model = %q, want %q", binding.Model, "gpt-5.4-mini")
	}
	if binding.ThinkingMode != "high" {
		t.Fatalf("Load() thinking_mode = %q, want %q", binding.ThinkingMode, "high")
	}
	if !binding.FastEnabled {
		t.Fatal("Load() fast_enabled = false, want true")
	}
	if !binding.LastUserMessageAt.Equal(lastUserMessageAt) {
		t.Fatalf("Load() last_user_message_at = %v, want %v", binding.LastUserMessageAt, lastUserMessageAt)
	}
	if binding.Metadata["force_fresh_thread"] != nil {
		t.Fatalf("Load() force_fresh_thread = %#v, want cleared", binding.Metadata["force_fresh_thread"])
	}
	if binding.Metadata["recovery_mode"] != nil {
		t.Fatalf("Load() recovery_mode = %#v, want cleared", binding.Metadata["recovery_mode"])
	}
	if binding.Metadata["last_compaction_at"] != nil {
		t.Fatalf("Load() last_compaction_at = %#v, want cleared", binding.Metadata["last_compaction_at"])
	}
}

func TestRunner_ReadStatusMergesBindingAndClientState(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:          "telegram:chat-1:coder",
		ThreadID:     "thr_existing",
		Model:        "gpt-5.4",
		ThinkingMode: "medium",
		FastEnabled:  true,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := &fakeRunnerClient{
		status: ClientStatus{
			Started:    true,
			TurnActive: true,
			Recovery: RecoveryStatus{
				Mode: "resume_after_restart",
			},
		},
		models: []ModelCatalogEntry{{ID: "gpt-5.4"}, {ID: "gpt-5.4-mini"}},
	}

	runner := NewRunner(client, store)
	status, err := runner.ReadStatus(context.Background(), "telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}

	if status.ThreadID != "thr_existing" {
		t.Fatalf("ReadStatus() thread_id = %q, want %q", status.ThreadID, "thr_existing")
	}
	if status.Model != "gpt-5.4" {
		t.Fatalf("ReadStatus() model = %q, want %q", status.Model, "gpt-5.4")
	}
	if status.ThinkingMode != "medium" {
		t.Fatalf("ReadStatus() thinking_mode = %q, want %q", status.ThinkingMode, "medium")
	}
	if !status.FastEnabled {
		t.Fatal("ReadStatus() fast_enabled = false, want true")
	}
	if !status.ClientStarted || !status.TurnActive {
		t.Fatalf("ReadStatus() client state = %+v, want started active", status)
	}
	if status.Recovery.Mode != "resume_after_restart" {
		t.Fatalf("ReadStatus() recovery = %+v, want mode resume_after_restart", status.Recovery)
	}
	if !slices.Equal(status.KnownModels, []string{"gpt-5.4", "gpt-5.4-mini"}) {
		t.Fatalf("ReadStatus() known_models = %v, want %v", status.KnownModels, []string{"gpt-5.4", "gpt-5.4-mini"})
	}
}

func TestRunner_ReadStatusProjectsContinuityMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 12, 15, 0, 0, 0, time.UTC)
	lastCompactionAt := now.Add(-2 * time.Minute)
	store := NewBindingStore(t.TempDir())
	if err := store.Save(Binding{
		Key:               "telegram:chat-1:coder",
		ThreadID:          "thr_existing",
		Model:             "gpt-5.4",
		ThinkingMode:      "high",
		FastEnabled:       true,
		LastUserMessageAt: now,
		Metadata: map[string]any{
			"force_fresh_thread": true,
			"last_compaction_at": lastCompactionAt.Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	runner := NewRunner(&fakeRunnerClient{}, store)
	status, err := runner.ReadStatus(context.Background(), "telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}

	if !status.ForceFreshThread {
		t.Fatal("ReadStatus() force_fresh_thread = false, want true")
	}
	if !status.LastUserMessageAt.Equal(now) {
		t.Fatalf("ReadStatus() last_user_message_at = %v, want %v", status.LastUserMessageAt, now)
	}
	if !status.LastCompactionAt.Equal(lastCompactionAt) {
		t.Fatalf("ReadStatus() last_compaction_at = %v, want %v", status.LastCompactionAt, lastCompactionAt)
	}
}

type fakeRunnerClient struct {
	startErr        error
	resumeErr       error
	resumeErrs      []error
	startThreadID   string
	runErr          error
	assistantChunks []string
	dynamicTools    []DynamicToolDefinition
	account         AccountSnapshot
	rateLimits      []RateLimitSnapshot

	resumedWith       string
	runThreadID       string
	runInput          string
	started           bool
	resumeCalls       int
	restartCalls      int
	compactedThreadID string
	runReq            RunTurnRequest
	models            []ModelCatalogEntry
	status            ClientStatus
	startCalls        int
}

func (c *fakeRunnerClient) Start(context.Context) error {
	c.startCalls++
	return c.startErr
}

func (c *fakeRunnerClient) ResumeThread(_ context.Context, threadID string, dynamicTools []DynamicToolDefinition) error {
	c.resumeCalls++
	c.resumedWith = threadID
	c.dynamicTools = dynamicTools
	if len(c.resumeErrs) > 0 {
		err := c.resumeErrs[0]
		c.resumeErrs = c.resumeErrs[1:]
		return err
	}
	return c.resumeErr
}

func (c *fakeRunnerClient) StartThread(_ context.Context, _ string, dynamicTools []DynamicToolDefinition) (string, error) {
	c.started = true
	c.dynamicTools = dynamicTools
	return c.startThreadID, c.startErr
}

func (c *fakeRunnerClient) RunTextTurn(_ context.Context, req RunTurnRequest) (string, error) {
	c.runReq = req
	c.runThreadID = req.ThreadID
	c.runInput = req.InputText

	content := ""
	for _, chunk := range c.assistantChunks {
		content += chunk
		if req.OnChunk != nil {
			req.OnChunk(chunk)
		}
	}

	return content, c.runErr
}

func (c *fakeRunnerClient) Restart(context.Context) error {
	c.restartCalls++
	return nil
}

func (c *fakeRunnerClient) StartNativeCompaction(_ context.Context, threadID string) error {
	c.compactedThreadID = threadID
	return nil
}

func (c *fakeRunnerClient) ListModels(context.Context) ([]ModelCatalogEntry, error) {
	return append([]ModelCatalogEntry(nil), c.models...), nil
}

func (c *fakeRunnerClient) ReadAccount(context.Context, bool) (AccountSnapshot, error) {
	return c.account, nil
}

func (c *fakeRunnerClient) ReadRateLimits(context.Context) ([]RateLimitSnapshot, error) {
	return append([]RateLimitSnapshot(nil), c.rateLimits...), nil
}

func (c *fakeRunnerClient) Close() error {
	return nil
}

func (c *fakeRunnerClient) Status() ClientStatus {
	return c.status
}
