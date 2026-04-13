package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func mustReply(t *testing.T) (func(string) error, *string) {
	t.Helper()
	var reply string
	return func(text string) error {
		reply = text
		return nil
	}, &reply
}

func TestBuiltinDefinitions_IncludesPhase3RuntimeCommands(t *testing.T) {
	defs := BuiltinDefinitions()

	want := map[string]bool{
		"set":     false,
		"fast":    false,
		"compact": false,
		"status":  false,
		"reset":   false,
	}
	for _, def := range defs {
		if _, ok := want[def.Name]; ok {
			want[def.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing /%s builtin definition", name)
		}
	}
}

func TestSetModel_UsesRuntimeCallback(t *testing.T) {
	rt := &Runtime{
		SetModel: func(value string) (string, error) {
			if value != "gpt-4" {
				t.Fatalf("SetModel value=%q, want gpt-4", value)
			}
			return "gpt-5.4", nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/set model gpt-4",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if *reply != "Model switched from gpt-5.4 to gpt-4" {
		t.Fatalf("reply=%q, want model switch confirmation", *reply)
	}
}

func TestSetThinking_UsesRuntimeCallback(t *testing.T) {
	rt := &Runtime{
		SetThinking: func(value string) (string, error) {
			if value != "deep" {
				t.Fatalf("SetThinking value=%q, want deep", value)
			}
			return "auto", nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/set thinking deep",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if *reply != "Thinking mode switched from auto to deep" {
		t.Fatalf("reply=%q, want thinking confirmation", *reply)
	}
}

func TestFast_TogglesRuntimeState(t *testing.T) {
	calls := 0
	rt := &Runtime{
		ToggleFast: func() (bool, error) {
			calls++
			return true, nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/fast",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if calls != 1 {
		t.Fatalf("ToggleFast calls=%d, want 1", calls)
	}
	if *reply != "Fast mode enabled" {
		t.Fatalf("reply=%q, want fast enabled confirmation", *reply)
	}
}

func TestCompact_UsesRuntimeCallback(t *testing.T) {
	called := false
	rt := &Runtime{
		CompactThread: func() error {
			called = true
			return nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/compact",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !called {
		t.Fatal("CompactThread was not called")
	}
	if *reply != "Thread compacted." {
		t.Fatalf("reply=%q, want compact confirmation", *reply)
	}
}

func TestStatus_UsesRuntimeSnapshot(t *testing.T) {
	rt := &Runtime{
		ReadStatus: func() StatusSnapshot {
			return StatusSnapshot{
				ThreadID:      "thread-123",
				Model:         "gpt-5.4",
				ThinkingMode:  "deep",
				FastEnabled:   true,
				RecoveryState: "fresh",
			}
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/status",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !strings.Contains(*reply, "Thread ID: thread-123") {
		t.Fatalf("reply=%q, want thread id", *reply)
	}
	if !strings.Contains(*reply, "Model: gpt-5.4") {
		t.Fatalf("reply=%q, want model", *reply)
	}
	if !strings.Contains(*reply, "Thinking: deep") {
		t.Fatalf("reply=%q, want thinking", *reply)
	}
	if !strings.Contains(*reply, "Fast: enabled") {
		t.Fatalf("reply=%q, want fast state", *reply)
	}
}

func TestReset_UsesRuntimeCallbackAndDoesNotClearHistory(t *testing.T) {
	resetCalled := false
	clearCalled := false
	rt := &Runtime{
		ResetThread: func() error {
			resetCalled = true
			return nil
		},
		ClearHistory: func() error {
			clearCalled = true
			return nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/reset",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !resetCalled {
		t.Fatal("ResetThread was not called")
	}
	if clearCalled {
		t.Fatal("ClearHistory should not be called by /reset")
	}
	if *reply != "Thread reset." {
		t.Fatalf("reply=%q, want reset confirmation", *reply)
	}
}

func TestClear_StillOnlyClearsLocalHistory(t *testing.T) {
	resetCalled := false
	clearCalled := false
	rt := &Runtime{
		ResetThread: func() error {
			resetCalled = true
			return nil
		},
		ClearHistory: func() error {
			clearCalled = true
			return nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/clear",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !clearCalled {
		t.Fatal("ClearHistory was not called")
	}
	if resetCalled {
		t.Fatal("ResetThread should not be called by /clear")
	}
	if *reply != "Chat history cleared!" {
		t.Fatalf("reply=%q, want clear confirmation", *reply)
	}
}

func TestListModels_UsesRuntimeCatalog(t *testing.T) {
	rt := &Runtime{
		ListModels: func() []ModelInfo {
			return []ModelInfo{
				{Name: "gpt-5.4", Provider: "codex"},
				{Name: "gpt-5.4-mini", Provider: "codex"},
			}
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/list models",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !strings.Contains(*reply, "gpt-5.4") || !strings.Contains(*reply, "gpt-5.4-mini") {
		t.Fatalf("reply=%q, want model catalog", *reply)
	}
}

func TestShowModel_UsesRuntimeStatus(t *testing.T) {
	rt := &Runtime{
		ReadStatus: func() StatusSnapshot {
			return StatusSnapshot{Model: "gpt-5.4", Provider: "codex"}
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/show model",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if *reply != "Current Model: gpt-5.4 (Provider: codex)" {
		t.Fatalf("reply=%q, want status-backed model line", *reply)
	}
}

func TestSwitchModel_UsesRuntimeSetModel(t *testing.T) {
	rt := &Runtime{
		SetModel: func(value string) (string, error) {
			if value != "gpt-4" {
				t.Fatalf("SetModel value=%q, want gpt-4", value)
			}
			return "gpt-5.4", nil
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/switch model to gpt-4",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if *reply != "Switched model from gpt-5.4 to gpt-4" {
		t.Fatalf("reply=%q, want switch confirmation", *reply)
	}
}

func TestSetCommand_UsageForMissingSubcommand(t *testing.T) {
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), &Runtime{})

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/set",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !strings.HasPrefix(*reply, "Usage: /set [") {
		t.Fatalf("reply=%q, want usage message", *reply)
	}
}

func TestSetModel_ErrorIsReturned(t *testing.T) {
	rt := &Runtime{
		SetModel: func(value string) (string, error) {
			return "", fmt.Errorf("model not found")
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	replyFn, reply := mustReply(t)
	res := ex.Execute(context.Background(), Request{
		Text:  "/set model bad",
		Reply: replyFn,
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if *reply != "model not found" {
		t.Fatalf("reply=%q, want error message", *reply)
	}
}
