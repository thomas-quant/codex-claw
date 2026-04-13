# Phase 5: Codex Continuity And Native Compaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Codex-backed conversations survive restarts cleanly, roll over after long inactivity, bootstrap fresh threads from PicoClaw-owned context, and compact natively before the thread hits context failure.

**Architecture:** Keep lifecycle policy in `pkg/agent`, where PicoClaw already owns local session history and per-thread orchestration. `pkg/codexruntime` stays a bounded primitive layer for binding persistence, resume/start/compact operations, and runtime status; `pkg/providers` only forwards continuity instructions into that runtime.

**Tech Stack:** Go 1.25, Codex app-server over `stdio`, PicoClaw session/context pipeline, `go test`

---

## File Structure

### Create

- `pkg/agent/interactive_continuity.go` — continuity helpers for bootstrap seeding, rollover checks, and pre-turn compaction decisions
- `pkg/agent/interactive_continuity_test.go` — focused unit tests for bootstrap payload building and rollover heuristics

### Modify

- `pkg/providers/types.go` — expand interactive request/status contracts for bootstrap and continuity metadata
- `pkg/providers/codex_app_server_provider.go` — stop reducing fresh-thread runs to only the last user message
- `pkg/providers/codex_app_server_provider_test.go` — provider contract coverage for bootstrap text and status projection
- `pkg/codexruntime/runner.go` — bounded resume/restart/fresh-thread recovery and fresh-thread bootstrap input
- `pkg/codexruntime/runner_test.go` — recovery, fresh bootstrap, and metadata persistence coverage
- `pkg/codexruntime/status.go` — expose richer continuity metadata to the loop
- `pkg/codexruntime/binding_store.go` — persist any new continuity markers needed by rollover/recovery
- `pkg/codexruntime/binding_store_test.go` — binding persistence coverage for the new metadata
- `pkg/agent/context_budget.go` — add a threshold-friendly helper instead of only a boolean overflow check
- `pkg/agent/context_budget_test.go` — threshold helper coverage
- `pkg/agent/loop.go` — pre-turn continuity policy, proactive native compaction, and fresh-thread seeding
- `pkg/agent/loop_test.go` — narrow integration coverage for rollover, compaction, and bootstrap wiring

## Task 1: Extend The Interactive Runtime Contract For Continuity

**Files:**
- Modify: `pkg/providers/types.go`
- Modify: `pkg/providers/codex_app_server_provider.go`
- Modify: `pkg/providers/codex_app_server_provider_test.go`
- Modify: `pkg/codexruntime/status.go`

- [ ] **Step 1: Write provider-layer failing tests for bootstrap input and richer status**

Add cases to `pkg/providers/codex_app_server_provider_test.go` that assert:

```go
func TestCodexAppServerProvider_RunInteractiveTurn_ForwardsBootstrapInput(t *testing.T) {
	runner := &fakeCodexAppServerRunner{
		runResult: codexruntime.RunResult{Content: "ok", ThreadID: "thr_1"},
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
		BootstrapInput: "SYSTEM\nUSER:first\nASSISTANT:reply\nUSER:current",
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}
	if runner.lastRunRequest.InputText != "SYSTEM\nUSER:first\nASSISTANT:reply\nUSER:current" {
		t.Fatalf("RunInteractiveTurn() input = %q", runner.lastRunRequest.InputText)
	}
}

func TestCodexAppServerProvider_ReadThreadStatus_ProjectsContinuityFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	provider := NewCodexAppServerProvider(&fakeCodexAppServerRunner{
		status: codexruntime.RuntimeStatusSnapshot{
			ThreadID:          "thr_1",
			Model:             "gpt-5.4",
			ThinkingMode:      "high",
			FastEnabled:       true,
			LastUserMessageAt: now,
			LastCompactionAt:  now.Add(-2 * time.Minute),
			ForceFreshThread:  true,
			Recovery:          codexruntime.RecoveryStatus{Mode: "fresh"},
		},
	})

	status, err := provider.ReadThreadStatus(context.Background(), InteractiveThreadControlRequest{
		SessionKey: "session-1",
		AgentID:    "agent-7",
	})
	if err != nil {
		t.Fatalf("ReadThreadStatus() error = %v", err)
	}
	if status.LastUserMessageAt != now || !status.LastCompactionAt.Equal(now.Add(-2*time.Minute)) || !status.ForceFreshThread {
		t.Fatalf("ReadThreadStatus() continuity fields = %#v", status)
	}
}
```

- [ ] **Step 2: Run provider tests to confirm they fail on the current contract**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/providers -run 'TestCodexAppServerProvider_(RunInteractiveTurn_ForwardsBootstrapInput|ReadThreadStatus_ProjectsContinuityFields)' -count=1
```

Expected: FAIL because `InteractiveTurnRequest` and `InteractiveThreadStatus` do not yet carry the new fields.

- [ ] **Step 3: Add the continuity fields to the provider contract**

Modify `pkg/providers/types.go` so the interactive request and status surfaces can carry fresh-thread bootstrap input and orchestration metadata:

```go
type InteractiveControlRequest struct {
	ThinkingMode      string
	FastEnabled       bool
	LastUserMessageAt string
	ForceFreshThread  bool
}

type InteractiveTurnRequest struct {
	SessionKey     string
	AgentID        string
	Channel        string
	ChatID         string
	Model          string
	Messages       []Message
	Tools          []ToolDefinition
	Options        map[string]any
	BootstrapInput string
	Recovery       InteractiveRecoveryRequest
	Control        InteractiveControlRequest
	OnChunk        func(string)
	ExecuteTool    InteractiveToolExecutor
}

type InteractiveThreadStatus struct {
	ThreadID          string
	Model             string
	Provider          string
	ThinkingMode      string
	FastEnabled       bool
	RecoveryState     string
	LastUserMessageAt time.Time
	LastCompactionAt  time.Time
	ForceFreshThread  bool
}
```

- [ ] **Step 4: Forward the new fields through the Codex provider**

Update `pkg/providers/codex_app_server_provider.go` so it stops always deriving turn text from `lastUserMessageContent(req.Messages)` and instead prefers an explicit bootstrap payload:

```go
inputText := strings.TrimSpace(req.BootstrapInput)
if inputText == "" {
	inputText = lastUserMessageContent(req.Messages)
}

result, err := p.runner.RunTextTurn(ctx, codexruntime.RunRequest{
	BindingKey: interactiveBindingKey(req),
	Model:      req.Model,
	InputText:  inputText,
	Recovery: codexruntime.RecoveryRequest{
		AllowServerRestart: req.Recovery.AllowServerRestart,
		AllowResume:        req.Recovery.AllowResume,
	},
	Control: codexruntime.ControlRequest{
		ThinkingMode:      req.Control.ThinkingMode,
		FastEnabled:       req.Control.FastEnabled,
		LastUserMessageAt: req.Control.LastUserMessageAt,
		ForceFreshThread:  req.Control.ForceFreshThread,
	},
	DynamicTools:   mapDynamicTools(req.Tools),
	HandleToolCall: mapInteractiveToolExecutor(req.ExecuteTool),
	OnChunk:        req.OnChunk,
})
```

Also project the new continuity fields out of `RuntimeStatusSnapshot`.

- [ ] **Step 5: Run provider tests to verify the contract now passes**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/providers -run 'TestCodexAppServerProvider_(RunInteractiveTurn_ForwardsBootstrapInput|ReadThreadStatus_ProjectsContinuityFields|RunInteractiveTurn_ForwardsRequest)' -count=1
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/providers/types.go pkg/providers/codex_app_server_provider.go pkg/providers/codex_app_server_provider_test.go pkg/codexruntime/status.go
git commit -m "feat(codex): extend interactive continuity contract"
```

## Task 2: Teach The Runtime To Force Fresh Threads And Persist Continuity Metadata

**Files:**
- Modify: `pkg/codexruntime/runner.go`
- Modify: `pkg/codexruntime/runner_test.go`
- Modify: `pkg/codexruntime/status.go`
- Modify: `pkg/codexruntime/binding_store.go`
- Modify: `pkg/codexruntime/binding_store_test.go`

- [ ] **Step 1: Write failing runtime tests for forced fresh starts and bounded recovery**

Add tests to `pkg/codexruntime/runner_test.go` that pin the desired policy:

```go
func TestRunner_ForceFreshThreadSkipsResumeAndStartsNewThread(t *testing.T) {
	store := NewBindingStore(t.TempDir())
	_ = store.Save(Binding{Key: "telegram:chat-1:coder", ThreadID: "thr_old", Model: "gpt-5.4"})
	client := &fakeRunnerClient{startThreadID: "thr_new", assistantChunks: []string{"fresh"}}
	runner := NewRunner(client, store)

	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "bootstrap payload",
		Recovery:   RecoveryRequest{AllowResume: true, AllowServerRestart: true},
		Control:    ControlRequest{ForceFreshThread: true},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if got.ThreadID != "thr_new" || client.resumeCalls != 0 || !client.started {
		t.Fatalf("RunTextTurn() = %#v, resumeCalls=%d started=%v", got, client.resumeCalls, client.started)
	}
}

func TestRunner_ResumeFailureFallsBackToFreshWithSeededInput(t *testing.T) {
	store := NewBindingStore(t.TempDir())
	_ = store.Save(Binding{Key: "telegram:chat-1:coder", ThreadID: "thr_old", Model: "gpt-5.4"})
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
		Recovery:   RecoveryRequest{AllowResume: true, AllowServerRestart: true},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if client.resumeCalls != 2 || client.runInputText != "USER: old\nASSISTANT: old reply\nUSER: current" {
		t.Fatalf("resumeCalls=%d runInput=%q", client.resumeCalls, client.runInputText)
	}
}
```

Add binding/status tests for continuity markers:

```go
func TestBindingStore_ResetThreadPreservesRuntimeSettingsAndTimestamps(t *testing.T) {
	store := NewBindingStore(t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	_ = store.Save(Binding{
		Key:               "telegram:chat-1:coder",
		ThreadID:          "thr_old",
		Model:             "gpt-5.4-mini",
		ThinkingMode:      "medium",
		FastEnabled:       true,
		LastUserMessageAt: now,
		Metadata:          map[string]any{"last_compaction_at": now.Format(time.RFC3339Nano)},
	})
	if err := store.ResetThread("telegram:chat-1:coder"); err != nil {
		t.Fatalf("ResetThread() error = %v", err)
	}
	got, _, _ := store.Load("telegram:chat-1:coder")
	if got.ThreadID != "" || got.Model != "gpt-5.4-mini" || !got.FastEnabled || !got.LastUserMessageAt.Equal(now) {
		t.Fatalf("Load() after reset = %#v", got)
	}
}

func TestBuildRuntimeStatus_ProjectsContinuityMetadata(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	got := BuildRuntimeStatus(RuntimeStatusInput{
		Binding: Binding{
			Key:               "telegram:chat-1:coder",
			ThreadID:          "thr_old",
			LastUserMessageAt: now,
			Metadata:          map[string]any{"force_fresh_thread": true},
		},
	})
	if !got.ForceFreshThread || !got.LastUserMessageAt.Equal(now) {
		t.Fatalf("BuildRuntimeStatus() = %#v", got)
	}
}
```

- [ ] **Step 2: Run runtime tests to verify they fail first**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/codexruntime -run 'TestRunner_(ForceFreshThreadSkipsResumeAndStartsNewThread|ResumeFailureFallsBackToFreshWithSeededInput)|Test(BuildRuntimeStatus_ProjectsContinuityMetadata|BindingStore_ResetThreadPreservesRuntimeSettingsAndTimestamps)' -count=1
```

Expected: FAIL because the runner cannot force a fresh start and does not surface the extra continuity state yet.

- [ ] **Step 3: Extend the runner control path with a force-fresh flag**

Update `pkg/codexruntime/runner.go`:

```go
type ControlRequest struct {
	ThinkingMode      string
	FastEnabled       bool
	LastUserMessageAt string
	ForceFreshThread  bool
}

func (r *Runner) RunTextTurn(ctx context.Context, req RunRequest) (RunResult, error) {
	allowResume := req.Recovery.AllowResume && !req.Control.ForceFreshThread
	// existing binding load...
	if ok && threadID != "" && allowResume {
		// current bounded resume path
	}
	if threadID == "" {
		threadID, err = r.client.StartThread(ctx, req.Model, req.DynamicTools)
		// ...
	}
	// RunTextTurn still uses req.InputText, which the agent/provider now owns.
}
```

Persist continuity markers in binding metadata when saving:

```go
binding.Metadata["recovery_mode"] = opts.Recovery.Mode
binding.Metadata["restart_attempted"] = opts.Recovery.RestartAttempted
binding.Metadata["resume_attempted"] = opts.Recovery.ResumeAttempted
binding.Metadata["fell_back_to_fresh"] = opts.Recovery.FellBackToFresh
binding.Metadata["force_fresh_thread"] = req.Control.ForceFreshThread
```

Update `BuildRuntimeStatus(...)` so `RuntimeStatusSnapshot` includes:

```go
type RuntimeStatusSnapshot struct {
	BindingKey        string
	ThreadID          string
	Model             string
	ThinkingMode      string
	FastEnabled       bool
	LastUserMessageAt time.Time
	LastCompactionAt  time.Time
	ClientStarted     bool
	TurnActive        bool
	KnownModels       []string
	Recovery          RecoveryStatus
	ForceFreshThread  bool
}
```

- [ ] **Step 4: Keep binding reset narrow**

`pkg/codexruntime/binding_store.go` should keep `/reset` semantics intact:

```go
func (s *BindingStore) ResetThread(key string) error {
	// Clear ThreadID and per-thread recovery/compaction markers,
	// but preserve Model, ThinkingMode, FastEnabled, and LastUserMessageAt.
}
```

Do not turn `/reset` into a full binding delete.

- [ ] **Step 5: Run runtime package tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/codexruntime -run 'TestRunner_|TestBuildRuntimeStatus_|TestBindingStore_' -count=1
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/codexruntime/runner.go pkg/codexruntime/runner_test.go pkg/codexruntime/status.go pkg/codexruntime/binding_store.go pkg/codexruntime/binding_store_test.go
git commit -m "feat(codex): persist continuity metadata and force-fresh control"
```

## Task 3: Add Agent-Level Bootstrap And Rollover Helpers

**Files:**
- Create: `pkg/agent/interactive_continuity.go`
- Create: `pkg/agent/interactive_continuity_test.go`
- Modify: `pkg/agent/context_budget.go`
- Modify: `pkg/agent/context_budget_test.go`

- [ ] **Step 1: Write failing helper tests for bootstrap, rollover, and threshold checks**

Create `pkg/agent/interactive_continuity_test.go` with focused unit tests:

```go
func TestBuildInteractiveBootstrapInput_UsesRecentTurnsAndCurrentMessage(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply one"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "reply two"},
		{Role: "user", Content: "current"},
	}
	got := buildInteractiveBootstrapInput(messages, 2)
	if !strings.Contains(got, "USER: second") || !strings.Contains(got, "USER: current") {
		t.Fatalf("buildInteractiveBootstrapInput() = %q", got)
	}
}

func TestShouldForceFreshInteractiveThread_WhenInactiveForOverEightHours(t *testing.T) {
	status := providers.InteractiveThreadStatus{
		ThreadID:          "thr_1",
		LastUserMessageAt: time.Now().UTC().Add(-9 * time.Hour),
	}
	if !shouldForceFreshInteractiveThread(time.Now().UTC(), status) {
		t.Fatal("shouldForceFreshInteractiveThread() = false, want true")
	}
}

func TestShouldForceFreshInteractiveThread_StaysFalseForWarmThread(t *testing.T) {
	status := providers.InteractiveThreadStatus{
		ThreadID:          "thr_1",
		LastUserMessageAt: time.Now().UTC().Add(-2 * time.Hour),
	}
	if shouldForceFreshInteractiveThread(time.Now().UTC(), status) {
		t.Fatal("shouldForceFreshInteractiveThread() = true, want false")
	}
}
```

Add threshold tests to `pkg/agent/context_budget_test.go`:

```go
func TestRemainingContextPercent(t *testing.T) {
	messages := []providers.Message{{Role: "user", Content: strings.Repeat("hello ", 200)}}
	got := remainingContextPercent(4096, messages, nil, 512)
	if got <= 0 || got >= 100 {
		t.Fatalf("remainingContextPercent() = %d, want bounded percentage", got)
	}
}

func TestIsLowContextThreshold(t *testing.T) {
	messages := []providers.Message{{Role: "user", Content: strings.Repeat("hello ", 600)}}
	got := remainingContextPercent(4096, messages, nil, 512)
	if got > 30 {
		t.Fatalf("remainingContextPercent() = %d, want low-context threshold hit", got)
	}
}
```

- [ ] **Step 2: Run the helper tests and confirm they fail**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'Test(BuildInteractiveBootstrapInput|ShouldForceFreshInteractiveThread|RemainingContextPercent|IsLowContextThreshold)' -count=1
```

Expected: FAIL because the helper file and threshold functions do not exist yet.

- [ ] **Step 3: Add a focused continuity helper file**

Create `pkg/agent/interactive_continuity.go` with small, testable helpers:

```go
const interactiveThreadInactivityLimit = 8 * time.Hour

func shouldForceFreshInteractiveThread(now time.Time, status providers.InteractiveThreadStatus) bool {
	if status.LastUserMessageAt.IsZero() {
		return false
	}
	return now.UTC().Sub(status.LastUserMessageAt.UTC()) > interactiveThreadInactivityLimit
}

func buildInteractiveBootstrapInput(messages []providers.Message, recentTurns int) string {
	var b strings.Builder
	start := findRecentTurnStart(messages, recentTurns)
	for _, msg := range messages[start:] {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", role, strings.TrimSpace(msg.Content))
	}
	return strings.TrimSpace(b.String())
}
```

Keep the first version intentionally lean: text-only, transcript-derived, no extra summary generation.

- [ ] **Step 4: Add a threshold-friendly context helper**

Extend `pkg/agent/context_budget.go`:

```go
func remainingContextPercent(
	contextWindow int,
	messages []providers.Message,
	toolDefs []providers.ToolDefinition,
	maxTokens int,
) int {
	if contextWindow <= 0 {
		return 100
	}
	msgTokens := 0
	for _, m := range messages {
		msgTokens += EstimateMessageTokens(m)
	}
	used := msgTokens + EstimateToolDefsTokens(toolDefs) + maxTokens
	remaining := contextWindow - used
	if remaining < 0 {
		remaining = 0
	}
	return (remaining * 100) / contextWindow
}
```

This helper will drive proactive compaction in the loop without changing non-Codex behavior.

- [ ] **Step 5: Run the helper tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'Test(BuildInteractiveBootstrapInput|ShouldForceFreshInteractiveThread|RemainingContextPercent|IsLowContextThreshold)' -count=1
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/agent/interactive_continuity.go pkg/agent/interactive_continuity_test.go pkg/agent/context_budget.go pkg/agent/context_budget_test.go
git commit -m "feat(agent): add interactive continuity helpers"
```

## Task 4: Wire Bootstrap, Rollover, And Proactive Native Compaction Into The Agent Loop

**Files:**
- Modify: `pkg/agent/loop.go`
- Modify: `pkg/agent/loop_test.go`
- Modify: `pkg/providers/types.go`
- Modify: `pkg/providers/codex_app_server_provider.go`

- [ ] **Step 1: Write failing integration tests around the loop policy**

Add loop tests that pin the desired orchestration:

```go
func TestAgentLoop_InteractiveProviderBootstrapsFreshThreadFromHistory(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	provider := &interactiveLoopProvider{
		finalContent: "done",
		status:       providers.InteractiveThreadStatus{},
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	_, _ = al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "first",
	})
	_, _ = al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "second",
	})

	_, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "current",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	_, _, reqs, _ := provider.snapshot()
	if len(reqs) != 1 || !strings.Contains(reqs[0].BootstrapInput, "USER: current") {
		t.Fatalf("BootstrapInput = %q", reqs[0].BootstrapInput)
	}
}

func TestAgentLoop_InteractiveProviderForcesFreshThreadAfterEightHours(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	provider := &interactiveLoopProvider{
		finalContent: "done",
		status: providers.InteractiveThreadStatus{
			ThreadID:          "thr_123",
			Model:             "gpt-5.4",
			LastUserMessageAt: time.Now().UTC().Add(-9 * time.Hour),
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	_, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "current",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	_, _, reqs, _ := provider.snapshot()
	if len(reqs) != 1 || !reqs[0].Control.ForceFreshThread {
		t.Fatalf("ForceFreshThread = %#v", reqs)
	}
}

func TestAgentLoop_InteractiveProviderCompactsBeforeLowContextTurn(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			Codex: config.RuntimeCodexConfig{
				AutoCompactThresholdPercent: 30,
			},
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	provider := &interactiveLoopProvider{
		finalContent: "done",
		status: providers.InteractiveThreadStatus{
			ThreadID: "thr_123",
			Model:    "gpt-5.4",
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	_, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  strings.Repeat("hello ", 4000),
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if provider.compactCalls != 1 || len(provider.updates) != 1 {
		t.Fatalf("compactCalls=%d updates=%d", provider.compactCalls, len(provider.updates))
	}
}
```

Keep the assertions narrow:
- bootstrap input is non-empty and contains recent turn text
- `ForceFreshThread` flips on rollover
- `CompactThread()` fires before `RunInteractiveTurn()` when the thread is low on context

- [ ] **Step 2: Run the focused loop tests and confirm they fail**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestAgentLoop_InteractiveProvider(BootstrapsFreshThreadFromHistory|ForcesFreshThreadAfterEightHours|CompactsBeforeLowContextTurn)' -count=1
```

Expected: FAIL because the loop does not yet build bootstrap payloads or do proactive native compaction.

- [ ] **Step 3: Build continuity policy into the interactive branch of `processMessage`**

In `pkg/agent/loop.go`, keep the current status read but turn it into a continuity decision:

```go
status, statusErr := runtimeController.ReadThreadStatus(statusCtx, threadControlReq)
// ...
forceFreshThread := shouldForceFreshInteractiveThread(time.Now().UTC(), status)
	bootstrapInput := ""
if status.ThreadID == "" || forceFreshThread {
	bootstrapInput = buildInteractiveBootstrapInput(callMessages, 3)
}

interactiveControl := providers.InteractiveControlRequest{
	LastUserMessageAt: time.Now().UTC().Format(time.RFC3339Nano),
	ThinkingMode:      resolvedThinking,
	FastEnabled:       resolvedFast,
	ForceFreshThread:  forceFreshThread,
}
```

Then pass `BootstrapInput` into `InteractiveTurnRequest`.

- [ ] **Step 4: Add proactive native compaction before the interactive call**

Still in `pkg/agent/loop.go`, use the new threshold helper only for interactive Codex threads:

```go
remainingPct := remainingContextPercent(ts.agent.ContextWindow, callMessages, providerToolDefs, ts.agent.MaxTokens)
thresholdPct := al.cfg.Runtime.Codex.AutoCompactThresholdPercent
if threadController != nil && status.ThreadID != "" && thresholdPct > 0 && remainingPct <= thresholdPct {
	if err := threadController.CompactThread(turnCtx, threadControlReq); err != nil {
		logger.WarnCF("agent", "Proactive interactive compact failed", map[string]any{
			"session_key": ts.sessionKey,
			"error":       err.Error(),
		})
	}
}
```

This must happen before `RunInteractiveTurn(...)`, not as a mid-turn retry.

- [ ] **Step 5: Preserve non-Codex behavior**

Do not alter:

```go
if isOverContextBudget(ts.agent.ContextWindow, messages, toolDefs, ts.agent.MaxTokens) {
	// existing generic context manager path
}
```

The new proactive compact hook should be additive for the interactive runtime path, not a rewrite of the generic budget flow.

- [ ] **Step 6: Run the focused agent tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestAgentLoop_InteractiveProvider(BootstrapsFreshThreadFromHistory|ForcesFreshThreadAfterEightHours|CompactsBeforeLowContextTurn|PassesThreadRuntimeControl)|TestProcessMessage_ContextOverflow_InteractiveProviderUsesNativeCompaction' -count=1
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/agent/loop.go pkg/agent/loop_test.go pkg/providers/types.go pkg/providers/codex_app_server_provider.go
git commit -m "feat(agent): add codex continuity and proactive compaction"
```

## Task 5: Focused Verification And Follow-Up Capture

**Files:**
- Modify only files already touched in Tasks 1-4
- Modify: `followups.md`

- [ ] **Step 1: Run package-focused verification for the full phase**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/codexruntime ./pkg/providers ./pkg/agent -count=1
```

Expected: PASS

- [ ] **Step 2: Record the remaining follow-ons in `followups.md`**

Add or keep these items:

```markdown
- replace transcript-derived fresh-thread seeding with a better handoff-summary policy once memory strategy is settled
- deduplicate the interactive and legacy tool-execution paths in `pkg/agent`
- consider surfacing richer continuity fields in CLI/channel status output only after the runtime shape settles
- decide whether proactive compaction should eventually use live Codex-reported context signals instead of only local token estimates
```

- [ ] **Step 3: Commit**

```bash
git add followups.md pkg/codexruntime pkg/providers pkg/agent
git commit -m "test(codex): verify phase 5 continuity runtime"
```

## Worker Split

Use disjoint write sets and keep review tight:

1. Runtime worker
   - owns `pkg/codexruntime/*`
   - owns the status and binding metadata slice

2. Provider contract worker
   - owns `pkg/providers/types.go`
   - owns `pkg/providers/codex_app_server_provider.go`
   - owns `pkg/providers/codex_app_server_provider_test.go`

3. Agent policy worker
   - owns `pkg/agent/interactive_continuity.go`
   - owns `pkg/agent/context_budget.go`
   - owns `pkg/agent/loop.go`
   - owns matching tests

Shared contract between workers:

- bootstrap a fresh Codex thread from PicoClaw-owned context, not only the last user message
- force a fresh thread after more than 8 hours of inactivity per `(channel thread, agent id)`
- keep runtime settings across rollover and `/reset`
- compact proactively only between turns
- preserve non-Codex behavior outside the interactive path
