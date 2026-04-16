package providers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/thomas-quant/codex-claw/pkg/codexruntime"
)

func TestCodexAppServerProvider_RunInteractiveTurn_ForwardsRequest(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		result: codexruntime.RunResult{
			Content:  "assistant reply",
			ThreadID: "thread-123",
		},
	}
	provider := NewCodexAppServerProvider(runner)

	var chunks []string
	resp, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
		Channel:    "telegram",
		ChatID:     "chat-99",
		Model:      "gpt-5.4",
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "first user message"},
			{Role: "assistant", Content: "assistant reply"},
			{Role: "user", Content: "last user message"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: ToolFunctionDefinition{
					Name:        "lookup_weather",
					Description: "Looks up weather",
					Parameters: map[string]any{
						"type": "object",
					},
				},
			},
		},
		Options: map[string]any{
			"mode": "fast",
		},
		Recovery: InteractiveRecoveryRequest{
			AllowServerRestart: true,
			AllowResume:        true,
		},
		Control: InteractiveControlRequest{
			ThinkingMode:      "high",
			FastEnabled:       true,
			FastEnabledSet:    true,
			LastUserMessageAt: "2026-04-13T10:00:00Z",
			ForceFreshThread:  true,
		},
		OnChunk: func(chunk string) {
			chunks = append(chunks, chunk)
		},
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}

	if resp == nil {
		t.Fatal("RunInteractiveTurn() response is nil")
	}
	if resp.Content != "assistant reply" {
		t.Fatalf("RunInteractiveTurn() content = %q, want %q", resp.Content, "assistant reply")
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("RunInteractiveTurn() finish_reason = %q, want %q", resp.FinishReason, "stop")
	}

	wantReq := codexruntime.RunRequest{
		BindingKey: "session-1:agent-7",
		Model:      "gpt-5.4",
		InputText:  "last user message",
		DynamicTools: []codexruntime.DynamicToolDefinition{
			{
				Name:        "lookup_weather",
				Description: "Looks up weather",
				InputSchema: map[string]any{
					"type": "object",
				},
			},
		},
		OnChunk: runner.gotReq.OnChunk,
	}
	if runner.gotReq.BindingKey != wantReq.BindingKey {
		t.Fatalf("runner binding_key = %q, want %q", runner.gotReq.BindingKey, wantReq.BindingKey)
	}
	if runner.gotReq.Model != wantReq.Model {
		t.Fatalf("runner model = %q, want %q", runner.gotReq.Model, wantReq.Model)
	}
	if runner.gotReq.InputText != wantReq.InputText {
		t.Fatalf("runner input_text = %q, want %q", runner.gotReq.InputText, wantReq.InputText)
	}
	if !runner.gotReq.Recovery.AllowServerRestart || !runner.gotReq.Recovery.AllowResume {
		t.Fatalf("runner recovery = %#v, want restart+resume enabled", runner.gotReq.Recovery)
	}
	if runner.gotReq.Control.ThinkingMode != "high" || !runner.gotReq.Control.FastEnabled || !runner.gotReq.Control.FastEnabledSet || runner.gotReq.Control.LastUserMessageAt != "2026-04-13T10:00:00Z" {
		t.Fatalf("runner control = %#v, want forwarded control metadata", runner.gotReq.Control)
	}
	if !runner.gotReq.Control.ForceFreshThread {
		t.Fatalf("runner control.force_fresh_thread = %v, want true", runner.gotReq.Control.ForceFreshThread)
	}
	if runner.gotReq.OnChunk == nil {
		t.Fatal("runner on_chunk is nil")
	}
	if !slices.EqualFunc(runner.gotReq.DynamicTools, wantReq.DynamicTools, func(a, b codexruntime.DynamicToolDefinition) bool {
		return a.Name == b.Name && a.Description == b.Description && fmt.Sprint(a.InputSchema) == fmt.Sprint(b.InputSchema)
	}) {
		t.Fatalf("runner dynamic_tools = %#v, want %#v", runner.gotReq.DynamicTools, wantReq.DynamicTools)
	}

	if !slices.Equal(chunks, []string{"chunk-1", "chunk-2"}) {
		t.Fatalf("streamed chunks = %v, want %v", chunks, []string{"chunk-1", "chunk-2"})
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_ForwardsBootstrapInput(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		result: codexruntime.RunResult{
			Content:  "assistant reply",
			ThreadID: "thread-123",
		},
	}
	provider := NewCodexAppServerProvider(runner)

	_, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
		Model:      "gpt-5.4",
		Messages: []Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "reply"},
			{Role: "user", Content: "current"},
		},
		RecoveryBootstrapInput: "USER:first\nASSISTANT:reply\nUSER:current",
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}

	if runner.gotReq.InputText != "USER:first\nASSISTANT:reply\nUSER:current" {
		t.Fatalf("RunInteractiveTurn() input_text = %q, want bootstrap payload", runner.gotReq.InputText)
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_PreservesBootstrapInputVerbatim(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		result: codexruntime.RunResult{
			Content:  "assistant reply",
			ThreadID: "thread-123",
		},
	}
	provider := NewCodexAppServerProvider(runner)

	bootstrapInput := "  USER:current\n\n"
	_, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		SessionKey:             "session-1",
		AgentID:                "agent-7",
		Model:                  "gpt-5.4",
		RecoveryBootstrapInput: bootstrapInput,
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}

	if runner.gotReq.InputText != bootstrapInput {
		t.Fatalf("RunInteractiveTurn() input_text = %q, want %q", runner.gotReq.InputText, bootstrapInput)
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_ErrorsWithoutBootstrapInputOrUserMessage(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{}
	provider := NewCodexAppServerProvider(runner)

	_, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
		Model:      "gpt-5.4",
		Messages: []Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "assistant"},
		},
	})
	if err == nil {
		t.Fatal("RunInteractiveTurn() error = nil, want error")
	}
	if runner.runTextTurnCalled {
		t.Fatalf("runner was called with request %#v, want no runner call", runner.gotReq)
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_ForwardsToolExecutor(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		invokeTool: codexruntime.ToolCallRequest{
			CallID: "call-1",
			Name:   "lookup_weather",
			Arguments: map[string]any{
				"city": "London",
			},
		},
	}
	provider := NewCodexAppServerProvider(runner)

	resp, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		Model:    "gpt-5.4",
		Messages: []Message{{Role: "user", Content: "hello"}},
		ExecuteTool: func(_ context.Context, call InteractiveToolCall) (InteractiveToolResult, error) {
			if call.CallID != "call-1" {
				t.Fatalf("ExecuteTool call_id = %q, want %q", call.CallID, "call-1")
			}
			if call.Name != "lookup_weather" {
				t.Fatalf("ExecuteTool name = %q, want %q", call.Name, "lookup_weather")
			}
			if call.Arguments["city"] != "London" {
				t.Fatalf("ExecuteTool arguments = %#v, want city=London", call.Arguments)
			}

			return InteractiveToolResult{
				Success: true,
				ContentItems: []InteractiveContentItem{
					{Type: "text", Text: "sunny"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RunInteractiveTurn() response is nil")
	}

	if runner.toolResult.Success != true {
		t.Fatalf("runner tool success = %v, want true", runner.toolResult.Success)
	}
	if len(runner.toolResult.Content) != 1 || runner.toolResult.Content[0].Text != "sunny" {
		t.Fatalf("runner tool result = %#v, want one sunny text item", runner.toolResult)
	}
}

func TestCodexAppServerProvider_Chat_DelegatesToInteractiveTurn(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		result: codexruntime.RunResult{
			Content: "assistant reply",
		},
	}
	provider := NewCodexAppServerProvider(runner)

	resp, err := provider.Chat(
		context.Background(),
		[]Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "hello from chat"},
		},
		[]ToolDefinition{
			{
				Type: "function",
				Function: ToolFunctionDefinition{
					Name:        "lookup_weather",
					Description: "Looks up weather",
				},
			},
		},
		"gpt-5.4",
		map[string]any{"mode": "compat"},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if resp == nil {
		t.Fatal("Chat() response is nil")
	}
	if resp.Content != "assistant reply" {
		t.Fatalf("Chat() content = %q, want %q", resp.Content, "assistant reply")
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("Chat() finish_reason = %q, want %q", resp.FinishReason, "stop")
	}

	if runner.gotReq.BindingKey != "" {
		t.Fatalf("runner binding_key = %q, want empty", runner.gotReq.BindingKey)
	}
	if runner.gotReq.Model != "gpt-5.4" {
		t.Fatalf("runner model = %q, want %q", runner.gotReq.Model, "gpt-5.4")
	}
	if runner.gotReq.InputText != "hello from chat" {
		t.Fatalf("runner input_text = %q, want %q", runner.gotReq.InputText, "hello from chat")
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_ReturnsRunnerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("runner failed")
	provider := NewCodexAppServerProvider(&fakeCodexAppServerRunner{err: wantErr})

	_, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		Channel:  "telegram",
		ChatID:   "chat-99",
		AgentID:  "agent-7",
		Model:    "gpt-5.4",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunInteractiveTurn() error = %v, want %v", err, wantErr)
	}
}

func TestCodexAppServerProvider_CompactThreadDelegatesToRunner(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{}
	provider := NewCodexAppServerProvider(runner)

	if err := provider.CompactThread(context.Background(), InteractiveThreadControlRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
	}); err != nil {
		t.Fatalf("CompactThread() error = %v", err)
	}
	if runner.compactBindingKey != "session-1:agent-7" {
		t.Fatalf("CompactThread() binding_key = %q, want %q", runner.compactBindingKey, "session-1:agent-7")
	}
}

func TestCodexAppServerProvider_RuntimeControlsDelegateToRunner(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		models: []codexruntime.ModelCatalogEntry{
			{ID: "gpt-5.4", SpeedTier: "standard"},
			{ID: "gpt-5.4-mini", SpeedTier: "fast"},
		},
		status: codexruntime.RuntimeStatusSnapshot{
			ThreadID:     "thr_123",
			Model:        "gpt-5.4",
			ThinkingMode: "medium",
			FastEnabled:  true,
			Recovery: codexruntime.RecoveryStatus{
				Mode: "resumed",
			},
		},
		setModelOldValue:    "gpt-5.4",
		setThinkingOldValue: "medium",
		toggleFastValue:     false,
	}
	provider := NewCodexAppServerProvider(runner)
	req := InteractiveThreadControlRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
	}

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 2 || models[0].ID != "gpt-5.4" || models[1].ID != "gpt-5.4-mini" {
		t.Fatalf("ListModels() = %#v, want codex catalog entries", models)
	}

	status, err := provider.ReadThreadStatus(context.Background(), req)
	if err != nil {
		t.Fatalf("ReadThreadStatus() error = %v", err)
	}
	if status.ThreadID != "thr_123" || status.Model != "gpt-5.4" || status.ThinkingMode != "medium" || !status.FastEnabled {
		t.Fatalf("ReadThreadStatus() = %#v, want thread/model/thinking/fast state", status)
	}
	if status.RecoveryState != "resumed" {
		t.Fatalf("ReadThreadStatus() recovery_state = %q, want %q", status.RecoveryState, "resumed")
	}

	oldModel, err := provider.SetThreadModel(context.Background(), req, "gpt-5.4-mini")
	if err != nil {
		t.Fatalf("SetThreadModel() error = %v", err)
	}
	if oldModel != "gpt-5.4" {
		t.Fatalf("SetThreadModel() old = %q, want %q", oldModel, "gpt-5.4")
	}
	if runner.setModelBindingKey != "session-1:agent-7" || runner.setModelValue != "gpt-5.4-mini" {
		t.Fatalf("SetThreadModel() runner args = %q %q", runner.setModelBindingKey, runner.setModelValue)
	}

	oldThinking, err := provider.SetThreadThinking(context.Background(), req, "high")
	if err != nil {
		t.Fatalf("SetThreadThinking() error = %v", err)
	}
	if oldThinking != "medium" {
		t.Fatalf("SetThreadThinking() old = %q, want %q", oldThinking, "medium")
	}

	fastEnabled, err := provider.ToggleThreadFast(context.Background(), req)
	if err != nil {
		t.Fatalf("ToggleThreadFast() error = %v", err)
	}
	if fastEnabled {
		t.Fatal("ToggleThreadFast() = true, want false")
	}

	if err := provider.ResetThread(context.Background(), req); err != nil {
		t.Fatalf("ResetThread() error = %v", err)
	}
	if runner.resetBindingKey != "session-1:agent-7" {
		t.Fatalf("ResetThread() binding_key = %q, want %q", runner.resetBindingKey, "session-1:agent-7")
	}
}

func TestCodexAppServerProvider_ReadThreadStatus_ProjectsContinuityFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	lastCompactionAt := now.Add(-2 * time.Minute)
	runner := &fakeCodexAppServerRunner{
		status: codexruntime.RuntimeStatusSnapshot{
			ThreadID:          "thr_1",
			Model:             "gpt-5.4",
			ThinkingMode:      "high",
			FastEnabled:       true,
			LastUserMessageAt: now,
			LastCompactionAt:  lastCompactionAt,
			ForceFreshThread:  true,
			Recovery: codexruntime.RecoveryStatus{
				Mode: "fresh",
			},
		},
	}
	provider := NewCodexAppServerProvider(runner)

	status, err := provider.ReadThreadStatus(context.Background(), InteractiveThreadControlRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
	})
	if err != nil {
		t.Fatalf("ReadThreadStatus() error = %v", err)
	}

	if status.LastUserMessageAt != now {
		t.Fatalf("ReadThreadStatus() last_user_message_at = %v, want %v", status.LastUserMessageAt, now)
	}
	if !status.LastCompactionAt.Equal(lastCompactionAt) {
		t.Fatalf("ReadThreadStatus() last_compaction_at = %v, want %v", status.LastCompactionAt, lastCompactionAt)
	}
	if !status.ForceFreshThread {
		t.Fatalf("ReadThreadStatus() force_fresh_thread = %v, want true", status.ForceFreshThread)
	}
	if runner.readStatusBindingKey != "session-1:agent-7" {
		t.Fatalf("ReadThreadStatus() binding_key = %q, want %q", runner.readStatusBindingKey, "session-1:agent-7")
	}
}

func TestCodexAppServerProvider_ReadThreadStatus_ProjectsAccountFields(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{
		status: codexruntime.RuntimeStatusSnapshot{
			ThreadID:             "thr_1",
			Model:                "gpt-5.4",
			ActiveAccountAlias:   "alpha",
			AccountHealth:        "healthy",
			TelemetryFresh:       true,
			FiveHourRemainingPct: 88,
			WeeklyRemainingPct:   91,
			SwitchTrigger:        "soft_threshold_5h",
		},
	}
	provider := NewCodexAppServerProvider(runner)

	status, err := provider.ReadThreadStatus(context.Background(), InteractiveThreadControlRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
	})
	if err != nil {
		t.Fatalf("ReadThreadStatus() error = %v", err)
	}

	if status.ActiveAccountAlias != "alpha" {
		t.Fatalf("ReadThreadStatus() active_account_alias = %q, want %q", status.ActiveAccountAlias, "alpha")
	}
	if status.AccountHealth != "healthy" {
		t.Fatalf("ReadThreadStatus() account_health = %q, want %q", status.AccountHealth, "healthy")
	}
	if !status.TelemetryFresh {
		t.Fatal("ReadThreadStatus() telemetry_fresh = false, want true")
	}
	if status.FiveHourRemainingPct != 88 {
		t.Fatalf("ReadThreadStatus() five_hour_remaining_pct = %d, want %d", status.FiveHourRemainingPct, 88)
	}
	if status.WeeklyRemainingPct != 91 {
		t.Fatalf("ReadThreadStatus() weekly_remaining_pct = %d, want %d", status.WeeklyRemainingPct, 91)
	}
	if status.SwitchTrigger != "soft_threshold_5h" {
		t.Fatalf("ReadThreadStatus() switch_trigger = %q, want %q", status.SwitchTrigger, "soft_threshold_5h")
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_UsesLastUserMessage(t *testing.T) {
	t.Parallel()

	runner := &fakeCodexAppServerRunner{}
	provider := NewCodexAppServerProvider(runner)

	_, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
		Model:      "gpt-5.4",
		Messages: []Message{
			{Role: "user", Content: "first user message"},
			{Role: "assistant", Content: "assistant reply"},
			{Role: "tool", Content: "tool output"},
			{Role: "user", Content: "visible user message"},
			{Role: "assistant", Content: "draft reply"},
		},
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}

	if runner.gotReq.InputText != "visible user message" {
		t.Fatalf("runner input_text = %q, want %q", runner.gotReq.InputText, "visible user message")
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_BuildsFallbackBindingKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  InteractiveTurnRequest
		want string
	}{
		{
			name: "full tuple",
			req: InteractiveTurnRequest{
				Channel:  "telegram",
				ChatID:   "chat-99",
				AgentID:  "agent-7",
				Model:    "gpt-5.4",
				Messages: []Message{{Role: "user", Content: "hello"}},
			},
			want: "telegram:chat-99:agent-7",
		},
		{
			name: "missing chat id",
			req: InteractiveTurnRequest{
				Channel:  "telegram",
				AgentID:  "agent-7",
				Model:    "gpt-5.4",
				Messages: []Message{{Role: "user", Content: "hello"}},
			},
			want: "telegram::agent-7",
		},
		{
			name: "missing channel",
			req: InteractiveTurnRequest{
				ChatID:   "chat-99",
				AgentID:  "agent-7",
				Model:    "gpt-5.4",
				Messages: []Message{{Role: "user", Content: "hello"}},
			},
			want: ":chat-99:agent-7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeCodexAppServerRunner{}
			provider := NewCodexAppServerProvider(runner)

			_, err := provider.RunInteractiveTurn(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("RunInteractiveTurn() error = %v", err)
			}

			if runner.gotReq.BindingKey != tt.want {
				t.Fatalf("runner binding_key = %q, want %q", runner.gotReq.BindingKey, tt.want)
			}
		})
	}
}

func TestInteractiveBindingKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  InteractiveTurnRequest
		want string
	}{
		{
			name: "session key and agent",
			req: InteractiveTurnRequest{
				SessionKey: "session-1",
				AgentID:    "agent-7",
			},
			want: "session-1:agent-7",
		},
		{
			name: "session key without agent",
			req: InteractiveTurnRequest{
				SessionKey: "session-1",
			},
			want: "session-1",
		},
		{
			name: "preserves missing chat id slot",
			req: InteractiveTurnRequest{
				Channel: "telegram",
				AgentID: "agent-7",
			},
			want: "telegram::agent-7",
		},
		{
			name: "preserves missing channel slot",
			req: InteractiveTurnRequest{
				ChatID:  "chat-99",
				AgentID: "agent-7",
			},
			want: ":chat-99:agent-7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := interactiveBindingKey(tt.req); got != tt.want {
				t.Fatalf("interactiveBindingKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeCodexAppServerRunner struct {
	result     codexruntime.RunResult
	err        error
	invokeTool codexruntime.ToolCallRequest

	gotReq                codexruntime.RunRequest
	runTextTurnCalled     bool
	toolResult            codexruntime.ToolCallResult
	compactBindingKey     string
	models                []codexruntime.ModelCatalogEntry
	status                codexruntime.RuntimeStatusSnapshot
	readStatusBindingKey  string
	setModelBindingKey    string
	setModelValue         string
	setModelOldValue      string
	setThinkingBindingKey string
	setThinkingValue      string
	setThinkingOldValue   string
	toggleFastBindingKey  string
	toggleFastValue       bool
	resetBindingKey       string
}

func (f *fakeCodexAppServerRunner) RunTextTurn(_ context.Context, req codexruntime.RunRequest) (codexruntime.RunResult, error) {
	f.runTextTurnCalled = true
	f.gotReq = req

	if req.OnChunk != nil {
		req.OnChunk("chunk-1")
		req.OnChunk("chunk-2")
	}
	if req.HandleToolCall != nil && f.invokeTool.Name != "" {
		result, err := req.HandleToolCall(context.Background(), f.invokeTool)
		if err != nil {
			return codexruntime.RunResult{}, err
		}
		f.toolResult = result
	}

	return f.result, f.err
}

func (f *fakeCodexAppServerRunner) CompactThread(_ context.Context, bindingKey string) error {
	f.compactBindingKey = bindingKey
	return nil
}

func (f *fakeCodexAppServerRunner) ListModels(_ context.Context) ([]codexruntime.ModelCatalogEntry, error) {
	return append([]codexruntime.ModelCatalogEntry(nil), f.models...), nil
}

func (f *fakeCodexAppServerRunner) ReadStatus(_ context.Context, bindingKey string) (codexruntime.RuntimeStatusSnapshot, error) {
	f.readStatusBindingKey = bindingKey
	return f.status, nil
}

func (f *fakeCodexAppServerRunner) SetModel(_ context.Context, bindingKey, model string) (string, error) {
	f.setModelBindingKey = bindingKey
	f.setModelValue = model
	return f.setModelOldValue, nil
}

func (f *fakeCodexAppServerRunner) SetThinkingMode(_ context.Context, bindingKey, thinking string) (string, error) {
	f.setThinkingBindingKey = bindingKey
	f.setThinkingValue = thinking
	return f.setThinkingOldValue, nil
}

func (f *fakeCodexAppServerRunner) ToggleFast(_ context.Context, bindingKey string) (bool, error) {
	f.toggleFastBindingKey = bindingKey
	return f.toggleFastValue, nil
}

func (f *fakeCodexAppServerRunner) ResetThread(_ context.Context, bindingKey string) error {
	f.resetBindingKey = bindingKey
	return nil
}
