package codexruntime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBindingStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewBindingStore(dir)

	want := Binding{
		Key:               "telegram:chat-1:agent-coder",
		ThreadID:          "thr_123",
		AgentID:           "coder",
		Channel:           "telegram",
		ThreadKey:         "chat-1",
		Model:             "gpt-5.4",
		ThinkingMode:      "medium",
		FastEnabled:       true,
		LastUserMessageAt: time.Date(2026, time.April, 12, 10, 0, 0, 0, time.UTC),
		Metadata: map[string]any{
			"runtime": "codex",
		},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, ok, err := store.Load(want.Key)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() reported missing binding")
	}
	if got.ThreadID != want.ThreadID || got.Model != want.Model || !got.FastEnabled {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	if got.LastUserMessageAt != want.LastUserMessageAt {
		t.Fatalf("Load() last_user_message_at = %v, want %v", got.LastUserMessageAt, want.LastUserMessageAt)
	}
	if got.Metadata["runtime"] != "codex" {
		t.Fatalf("Load() metadata = %#v, want runtime=codex", got.Metadata)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("Load() timestamps should be set, got %#v", got)
	}
}

func TestBindingStore_SaveLoadWithSanitizedKey(t *testing.T) {
	dir := t.TempDir()
	store := NewBindingStore(dir)

	binding := Binding{
		Key:      "../telegram:chat/1\\agent",
		ThreadID: "thr_456",
	}

	if err := store.Save(binding); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, ok, err := store.Load(binding.Key)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() reported missing binding")
	}
	if got.ThreadID != binding.ThreadID {
		t.Fatalf("Load() thread_id = %q, want %q", got.ThreadID, binding.ThreadID)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ReadDir() entries = %d, want 1", len(entries))
	}
	name := entries[0].Name()
	if filepath.Base(name) != name {
		t.Fatalf("binding filename escaped root: %q", name)
	}
	if filepath.Ext(name) != ".json" {
		t.Fatalf("binding filename = %q, want .json suffix", name)
	}
}

func TestBindingStore_SaveOverwritesExistingBinding(t *testing.T) {
	dir := t.TempDir()
	store := NewBindingStore(dir)

	createdAt := time.Date(2026, time.April, 12, 9, 0, 0, 0, time.UTC)
	first := Binding{
		Key:               "telegram:chat-1:agent-coder",
		ThreadID:          "thr_old",
		Model:             "gpt-5.4",
		LastUserMessageAt: time.Date(2026, time.April, 12, 9, 30, 0, 0, time.UTC),
		Metadata: map[string]any{
			"session": "old",
		},
		CreatedAt: createdAt,
	}
	if err := store.Save(first); err != nil {
		t.Fatalf("Save(first) error = %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	second := Binding{
		Key:               first.Key,
		ThreadID:          "thr_new",
		Model:             "gpt-5.5",
		LastUserMessageAt: time.Date(2026, time.April, 12, 10, 15, 0, 0, time.UTC),
		Metadata: map[string]any{
			"session": "new",
			"attempt": float64(2),
		},
	}
	if err := store.Save(second); err != nil {
		t.Fatalf("Save(second) error = %v", err)
	}

	got, ok, err := store.Load(first.Key)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() reported missing binding")
	}
	if got.ThreadID != "thr_new" || got.Model != "gpt-5.5" {
		t.Fatalf("Load() overwrite did not persist new values: %#v", got)
	}
	if got.CreatedAt != createdAt {
		t.Fatalf("Load() created_at = %v, want %v", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.After(createdAt) {
		t.Fatalf("Load() updated_at = %v, want after %v", got.UpdatedAt, createdAt)
	}
	if got.LastUserMessageAt != second.LastUserMessageAt {
		t.Fatalf("Load() last_user_message_at = %v, want %v", got.LastUserMessageAt, second.LastUserMessageAt)
	}
	if got.Metadata["session"] != "new" || got.Metadata["attempt"] != float64(2) {
		t.Fatalf("Load() metadata = %#v, want overwritten values", got.Metadata)
	}
}

func TestBindingStore_MutatorsAndDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewBindingStore(dir)

	if err := store.Save(Binding{
		Key:               "telegram:chat-1:agent-coder",
		ThreadID:          "thr_123",
		Model:             "gpt-5.4",
		ThinkingMode:      "medium",
		FastEnabled:       false,
		LastUserMessageAt: time.Date(2026, time.April, 12, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	lastUserMessageAt := time.Date(2026, time.April, 12, 10, 30, 0, 0, time.UTC)
	if err := store.SetModel("telegram:chat-1:agent-coder", "gpt-5.4-mini"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if err := store.SetThinkingMode("telegram:chat-1:agent-coder", "high"); err != nil {
		t.Fatalf("SetThinkingMode() error = %v", err)
	}
	if err := store.SetFastEnabled("telegram:chat-1:agent-coder", true); err != nil {
		t.Fatalf("SetFastEnabled() error = %v", err)
	}
	if err := store.SetLastUserMessageAt("telegram:chat-1:agent-coder", lastUserMessageAt); err != nil {
		t.Fatalf("SetLastUserMessageAt() error = %v", err)
	}

	got, ok, err := store.Load("telegram:chat-1:agent-coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() reported missing binding")
	}
	if got.Model != "gpt-5.4-mini" {
		t.Fatalf("Load() model = %q, want %q", got.Model, "gpt-5.4-mini")
	}
	if got.ThinkingMode != "high" {
		t.Fatalf("Load() thinking_mode = %q, want %q", got.ThinkingMode, "high")
	}
	if !got.FastEnabled {
		t.Fatal("Load() fast_enabled = false, want true")
	}
	if got.LastUserMessageAt != lastUserMessageAt {
		t.Fatalf("Load() last_user_message_at = %v, want %v", got.LastUserMessageAt, lastUserMessageAt)
	}

	if err := store.Delete("telegram:chat-1:agent-coder"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Load("telegram:chat-1:agent-coder"); err != nil {
		t.Fatalf("Load() after Delete error = %v", err)
	} else if ok {
		t.Fatal("Load() after Delete reported binding present")
	}
}

func TestBindingStore_ResetThreadPreservesRuntimeSettingsAndTimestamps(t *testing.T) {
	t.Parallel()

	store := NewBindingStore(t.TempDir())
	now := time.Date(2026, time.April, 12, 16, 0, 0, 0, time.UTC)
	if err := store.Save(Binding{
		Key:               "telegram:chat-1:coder",
		ThreadID:          "thr_old",
		Model:             "gpt-5.4-mini",
		ThinkingMode:      "medium",
		FastEnabled:       true,
		LastUserMessageAt: now,
		Metadata: map[string]any{
			"recovery_mode":      "resumed",
			"restart_attempted":  true,
			"resume_attempted":   true,
			"fell_back_to_fresh": true,
			"force_fresh_thread": true,
			"last_compaction_at": now.Format(time.RFC3339Nano),
			"runtime":            "codex",
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := store.ResetThread("telegram:chat-1:coder"); err != nil {
		t.Fatalf("ResetThread() error = %v", err)
	}

	got, ok, err := store.Load("telegram:chat-1:coder")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() reported missing binding")
	}
	if got.ThreadID != "" {
		t.Fatalf("Load() thread_id = %q, want empty", got.ThreadID)
	}
	if got.Model != "gpt-5.4-mini" {
		t.Fatalf("Load() model = %q, want %q", got.Model, "gpt-5.4-mini")
	}
	if got.ThinkingMode != "medium" {
		t.Fatalf("Load() thinking_mode = %q, want %q", got.ThinkingMode, "medium")
	}
	if !got.FastEnabled {
		t.Fatal("Load() fast_enabled = false, want true")
	}
	if !got.LastUserMessageAt.Equal(now) {
		t.Fatalf("Load() last_user_message_at = %v, want %v", got.LastUserMessageAt, now)
	}
	if got.Metadata["runtime"] != "codex" {
		t.Fatalf("Load() runtime metadata = %#v, want preserved", got.Metadata["runtime"])
	}
	if got.Metadata["recovery_mode"] != nil {
		t.Fatalf("Load() recovery_mode = %#v, want cleared", got.Metadata["recovery_mode"])
	}
	if got.Metadata["restart_attempted"] != nil {
		t.Fatalf("Load() restart_attempted = %#v, want cleared", got.Metadata["restart_attempted"])
	}
	if got.Metadata["resume_attempted"] != nil {
		t.Fatalf("Load() resume_attempted = %#v, want cleared", got.Metadata["resume_attempted"])
	}
	if got.Metadata["fell_back_to_fresh"] != nil {
		t.Fatalf("Load() fell_back_to_fresh = %#v, want cleared", got.Metadata["fell_back_to_fresh"])
	}
	if got.Metadata["force_fresh_thread"] != nil {
		t.Fatalf("Load() force_fresh_thread = %#v, want cleared", got.Metadata["force_fresh_thread"])
	}
	if got.Metadata["last_compaction_at"] != nil {
		t.Fatalf("Load() last_compaction_at = %#v, want cleared", got.Metadata["last_compaction_at"])
	}
}
