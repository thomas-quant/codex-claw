# Codex Runtime Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current `codex exec` path with a minimal Codex app-server foundation that can initialize a persistent `stdio` client, persist per-thread Codex bindings, project only final assistant text, and run one text-only Codex turn through the existing agent loop.

**Architecture:** Keep Codex behind PicoClaw's provider abstraction, but add a Codex-specific interactive capability instead of pretending app-server is a stateless HTTP model. Build `pkg/codexruntime` as the transport, binding, and projector layer, then add a `codex/...` provider that the agent loop can prefer for streaming text turns.

**Tech Stack:** Go 1.25, newline-delimited JSON-RPC over `stdio`, existing `pkg/session` and `pkg/bus`, `go test`

---

## Scope Split

This spec is too wide for one safe execution plan. This plan is only phase 1:

- included: protocol types, binding store, event projector, app-server client, text-only runner, provider capability, agent-loop streaming hookup
- excluded: tool bridge, approval bridge, MCP allowlist enforcement, slash commands, config rewrite, launcher deletion, provider/channel pruning

Follow-on plans should handle:

1. Codex tool + approval bridge
2. Commands + persistent thread settings + compaction policy
3. Config rewrite + surface deletions

## File Structure

### Create

- `pkg/codexruntime/protocol.go` — app-server request, response, and notification types used by the runtime
- `pkg/codexruntime/binding_store.go` — disk-backed Codex thread binding store
- `pkg/codexruntime/binding_store_test.go` — binding round-trip and overwrite tests
- `pkg/codexruntime/projector.go` — final-assistant-only event projection logic
- `pkg/codexruntime/projector_test.go` — projector behavior tests
- `pkg/codexruntime/client.go` — persistent `stdio` app-server client with initialize handshake
- `pkg/codexruntime/client_test.go` — fake-transport handshake and request/response tests
- `pkg/codexruntime/runner.go` — start/resume/run orchestration for one text-only turn
- `pkg/codexruntime/runner_test.go` — resume/start fallback and streamed-text tests
- `pkg/providers/codex_app_server_provider.go` — provider wrapper around the Codex runtime
- `pkg/providers/codex_app_server_provider_test.go` — provider request forwarding tests

### Modify

- `pkg/providers/types.go` — add the Codex interactive provider capability contract
- `pkg/providers/factory_provider.go` — route `codex/...` models to the new provider
- `pkg/providers/factory_provider_test.go` — add `codex/...` factory coverage
- `pkg/agent/loop.go` — prefer interactive provider path and bus streaming when available
- `pkg/agent/loop_test.go` — add one focused streaming-path test

### Do Not Touch In Phase 1

- `pkg/tools`
- `pkg/mcp`
- `pkg/commands`
- `pkg/config`
- `web/`

## Task 1: Add Bindings And Final-Text Projection

**Files:**
- Create: `pkg/codexruntime/protocol.go`
- Create: `pkg/codexruntime/binding_store.go`
- Create: `pkg/codexruntime/binding_store_test.go`
- Create: `pkg/codexruntime/projector.go`
- Create: `pkg/codexruntime/projector_test.go`

- [ ] **Step 1: Write the failing binding-store and projector tests**

```go
package codexruntime

import "testing"

func TestBindingStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewBindingStore(dir)

	want := Binding{
		Key:          "telegram:chat-1:agent-coder",
		ThreadID:     "thr_123",
		AgentID:      "coder",
		Channel:      "telegram",
		ThreadKey:    "chat-1",
		Model:        "gpt-5.4",
		ThinkingMode: "medium",
		FastEnabled:  true,
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
}

func TestProjector_FinalAssistantOnly(t *testing.T) {
	p := NewProjector("thr_1", "turn_1")

	p.Apply(Notification{
		Method: "item/reasoning/textDelta",
		Params: ReasoningTextDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "reasoning_1",
			Text:     "hidden",
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_1",
			Delta:    "Hello",
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_1",
			Delta:    " world",
		},
	})

	if got := p.FinalAssistantText(); got != "Hello world" {
		t.Fatalf("FinalAssistantText() = %q, want %q", got, "Hello world")
	}
	if got := p.ReasoningText(); got != "hidden" {
		t.Fatalf("ReasoningText() = %q, want %q", got, "hidden")
	}
}
```

- [ ] **Step 2: Run the package tests to verify they fail**

Run: `go test ./pkg/codexruntime -run 'TestBindingStore_SaveLoadRoundTrip|TestProjector_FinalAssistantOnly' -count=1`

Expected: FAIL with errors like `undefined: NewBindingStore`, `undefined: Binding`, `undefined: NewProjector`

- [ ] **Step 3: Write the minimal binding-store and projector implementation**

```go
package codexruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Binding struct {
	Key          string    `json:"key"`
	ThreadID     string    `json:"thread_id"`
	AgentID      string    `json:"agent_id"`
	Channel      string    `json:"channel"`
	ThreadKey    string    `json:"thread_key"`
	Model        string    `json:"model,omitempty"`
	ThinkingMode string    `json:"thinking_mode,omitempty"`
	FastEnabled  bool      `json:"fast_enabled,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BindingStore struct {
	root string
}

func NewBindingStore(root string) *BindingStore {
	return &BindingStore{root: root}
}

func (s *BindingStore) Save(binding Binding) error {
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	binding.UpdatedAt = time.Now().UTC()

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	path := filepath.Join(s.root, sanitizeBindingKey(binding.Key)+".json")
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *BindingStore) Load(key string) (Binding, bool, error) {
	path := filepath.Join(s.root, sanitizeBindingKey(key)+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Binding{}, false, nil
	}
	if err != nil {
		return Binding{}, false, err
	}
	var binding Binding
	if err := json.Unmarshal(data, &binding); err != nil {
		return Binding{}, false, err
	}
	return binding, true, nil
}

func sanitizeBindingKey(key string) string {
	key = strings.ReplaceAll(key, ":", "_")
	key = strings.ReplaceAll(key, "/", "_")
	key = strings.ReplaceAll(key, "\\", "_")
	return key
}
```

```go
package codexruntime

import "strings"

type Projector struct {
	threadID string
	turnID   string

	assistant map[string]string
	reasoning strings.Builder
	lastItem  string
}

func NewProjector(threadID, turnID string) *Projector {
	return &Projector{
		threadID:  threadID,
		turnID:    turnID,
		assistant: make(map[string]string),
	}
}

func (p *Projector) Apply(n Notification) {
	switch n.Method {
	case "item/agentMessage/delta":
		params, ok := n.Params.(AgentMessageDeltaParams)
		if !ok || params.ThreadID != p.threadID || params.TurnID != p.turnID {
			return
		}
		p.assistant[params.ItemID] += params.Delta
		p.lastItem = params.ItemID
	case "item/reasoning/textDelta":
		params, ok := n.Params.(ReasoningTextDeltaParams)
		if !ok || params.ThreadID != p.threadID || params.TurnID != p.turnID {
			return
		}
		p.reasoning.WriteString(params.Text)
	}
}

func (p *Projector) FinalAssistantText() string {
	return strings.TrimSpace(p.assistant[p.lastItem])
}

func (p *Projector) ReasoningText() string {
	return strings.TrimSpace(p.reasoning.String())
}
```

- [ ] **Step 4: Run the tests again**

Run: `go test ./pkg/codexruntime -run 'TestBindingStore_SaveLoadRoundTrip|TestProjector_FinalAssistantOnly' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/codexruntime/protocol.go \
        pkg/codexruntime/binding_store.go \
        pkg/codexruntime/binding_store_test.go \
        pkg/codexruntime/projector.go \
        pkg/codexruntime/projector_test.go
git commit -m "feat(codex): add runtime bindings and projector"
```

## Task 2: Add App-Server Client And Text-Only Runner

**Files:**
- Create: `pkg/codexruntime/client.go`
- Create: `pkg/codexruntime/client_test.go`
- Create: `pkg/codexruntime/runner.go`
- Create: `pkg/codexruntime/runner_test.go`

- [ ] **Step 1: Write failing client and runner tests**

```go
package codexruntime

import (
	"context"
	"testing"
	"time"
)

func TestClient_InitializeHandshake(t *testing.T) {
	transport := newScriptedTransport(
		`{"id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := transport.LastMethod(); got != "initialize" {
		t.Fatalf("LastMethod() = %q, want %q", got, "initialize")
	}
	if !transport.SawNotification("initialized") {
		t.Fatal("expected initialized notification to be sent")
	}
}

func TestRunner_ResumeFallsBackToStart(t *testing.T) {
	fake := &fakeRunnerClient{
		resumeErr:       errResumeFailed,
		startThreadID:   "thr_new",
		assistantChunks: []string{"Hello", " world"},
	}
	store := NewBindingStore(t.TempDir())
	_ = store.Save(Binding{Key: "telegram:chat-1:coder", ThreadID: "thr_old"})

	runner := NewRunner(fake, store)
	got, err := runner.RunTextTurn(context.Background(), RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		InputText:  "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if got.Content != "Hello world" {
		t.Fatalf("RunTextTurn() content = %q, want %q", got.Content, "Hello world")
	}
	if !fake.started {
		t.Fatal("expected runner to fall back to thread/start")
	}
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./pkg/codexruntime -run 'TestClient_InitializeHandshake|TestRunner_ResumeFallsBackToStart' -count=1`

Expected: FAIL with errors like `undefined: NewClient`, `undefined: NewRunner`, `undefined: RunRequest`

- [ ] **Step 3: Implement the client and runner with fake-transport seams first**

```go
package codexruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

type Transport interface {
	Start(context.Context) (Conn, error)
}

type Conn interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close() error
}

type ClientOptions struct {
	RequestTimeout time.Duration
}

type Client struct {
	transport Transport
	opts      ClientOptions
	conn      Conn
	nextID    atomic.Int64
}

func NewClient(transport Transport, opts ClientOptions) *Client {
	return &Client{transport: transport, opts: opts}
}

func (c *Client) Start(ctx context.Context) error {
	conn, err := c.transport.Start(ctx)
	if err != nil {
		return err
	}
	c.conn = conn
	if err := c.write(ctx, map[string]any{
		"id":     1,
		"method": "initialize",
		"params": map[string]any{"clientInfo": map[string]any{"name": "picoclaw", "version": "0.0.0"}},
	}); err != nil {
		return err
	}
	if _, err := c.read(ctx); err != nil {
		return err
	}
	return c.write(ctx, map[string]any{"method": "initialized"})
}

func (c *Client) write(ctx context.Context, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, data)
}

func (c *Client) read(ctx context.Context) (map[string]any, error) {
	data, err := c.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return payload, nil
}
```

```go
package codexruntime

import (
	"context"
	"errors"
)

var errResumeFailed = errors.New("resume failed")

type RunRequest struct {
	BindingKey string
	Model      string
	InputText  string
	OnChunk    func(string)
}

type RunResult struct {
	Content  string
	ThreadID string
}

type RunnerClient interface {
	ResumeThread(context.Context, string) error
	StartThread(context.Context, string) (string, error)
	RunTextTurn(context.Context, string, string, func(string)) (string, error)
}

type Runner struct {
	client   RunnerClient
	bindings *BindingStore
}

func NewRunner(client RunnerClient, bindings *BindingStore) *Runner {
	return &Runner{client: client, bindings: bindings}
}

func (r *Runner) RunTextTurn(ctx context.Context, req RunRequest) (RunResult, error) {
	binding, ok, err := r.bindings.Load(req.BindingKey)
	if err != nil {
		return RunResult{}, err
	}

	threadID := binding.ThreadID
	if ok && threadID != "" {
		if err := r.client.ResumeThread(ctx, threadID); err != nil {
			threadID = ""
		}
	}
	if threadID == "" {
		threadID, err = r.client.StartThread(ctx, req.Model)
		if err != nil {
			return RunResult{}, err
		}
	}

	content, err := r.client.RunTextTurn(ctx, threadID, req.InputText, req.OnChunk)
	if err != nil {
		return RunResult{}, err
	}
	if err := r.bindings.Save(Binding{Key: req.BindingKey, ThreadID: threadID, Model: req.Model}); err != nil {
		return RunResult{}, err
	}
	return RunResult{Content: content, ThreadID: threadID}, nil
}
```

- [ ] **Step 4: Run the full package tests**

Run: `go test ./pkg/codexruntime -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/codexruntime/client.go \
        pkg/codexruntime/client_test.go \
        pkg/codexruntime/runner.go \
        pkg/codexruntime/runner_test.go
git commit -m "feat(codex): add app-server client and text runner"
```

## Task 3: Add Codex Provider Capability And Factory Route

**Files:**
- Modify: `pkg/providers/types.go`
- Create: `pkg/providers/codex_app_server_provider.go`
- Create: `pkg/providers/codex_app_server_provider_test.go`
- Modify: `pkg/providers/factory_provider.go`
- Modify: `pkg/providers/factory_provider_test.go`

- [ ] **Step 1: Write failing provider and factory tests**

```go
package providers

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestCreateProviderFromConfig_CodexAppServer(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "codex-main",
		Model:     "codex/gpt-5.4",
		Workspace: t.TempDir(),
	}

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if modelID != "gpt-5.4" {
		t.Fatalf("modelID = %q, want %q", modelID, "gpt-5.4")
	}
	if _, ok := provider.(*CodexAppServerProvider); !ok {
		t.Fatalf("provider type = %T, want *CodexAppServerProvider", provider)
	}
}

func TestCodexAppServerProvider_RunInteractiveTurn_ForwardsRequest(t *testing.T) {
	runner := &fakeInteractiveRunner{result: "done"}
	provider := NewCodexAppServerProvider(t.TempDir(), runner)

	resp, err := provider.RunInteractiveTurn(context.Background(), InteractiveTurnRequest{
		SessionKey: "telegram:chat-1",
		AgentID:    "coder",
		Channel:    "telegram",
		ChatID:     "chat-1",
		Model:      "gpt-5.4",
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("RunInteractiveTurn() error = %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("response content = %q, want %q", resp.Content, "done")
	}
}
```

- [ ] **Step 2: Run the provider tests to verify they fail**

Run: `go test ./pkg/providers -run 'TestCreateProviderFromConfig_CodexAppServer|TestCodexAppServerProvider_RunInteractiveTurn_ForwardsRequest' -count=1`

Expected: FAIL with errors like `undefined: CodexAppServerProvider`, `undefined: InteractiveTurnRequest`

- [ ] **Step 3: Add the provider capability and wire the factory**

```go
package providers

import "context"

type InteractiveTurnRequest struct {
	SessionKey string
	AgentID    string
	Channel    string
	ChatID     string
	Model      string
	Messages   []Message
	Tools      []ToolDefinition
	Options    map[string]any
	OnChunk    func(string)
}

type InteractiveProvider interface {
	LLMProvider
	RunInteractiveTurn(context.Context, InteractiveTurnRequest) (*LLMResponse, error)
}
```

```go
package providers

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/codexruntime"
)

type interactiveRunner interface {
	RunTextTurn(context.Context, codexruntime.RunRequest) (codexruntime.RunResult, error)
}

type CodexAppServerProvider struct {
	workspace string
	runner    interactiveRunner
}

func NewCodexAppServerProvider(workspace string, runner interactiveRunner) *CodexAppServerProvider {
	return &CodexAppServerProvider{workspace: workspace, runner: runner}
}

func (p *CodexAppServerProvider) GetDefaultModel() string { return "gpt-5.4" }

func (p *CodexAppServerProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	req := InteractiveTurnRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Options:  options,
	}
	return p.RunInteractiveTurn(ctx, req)
}

func (p *CodexAppServerProvider) RunInteractiveTurn(
	ctx context.Context,
	req InteractiveTurnRequest,
) (*LLMResponse, error) {
	var userText string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userText = strings.TrimSpace(req.Messages[i].Content)
			break
		}
	}

	result, err := p.runner.RunTextTurn(ctx, codexruntime.RunRequest{
		BindingKey: req.Channel + ":" + req.ChatID + ":" + req.AgentID,
		Model:      req.Model,
		InputText:  userText,
		OnChunk:    req.OnChunk,
	})
	if err != nil {
		return nil, err
	}
	return &LLMResponse{Content: result.Content, FinishReason: "stop"}, nil
}
```

```go
case "codex":
	bindings := codexruntime.NewBindingStore(filepath.Join(cfg.Workspace, "codex"))
	client := codexruntime.NewStdIOClient("codex", []string{"app-server", "--listen", "stdio://"}, cfg.RequestTimeout)
	runner := codexruntime.NewRunner(client, bindings)
	return NewCodexAppServerProvider(cfg.Workspace, runner), modelID, nil
```

- [ ] **Step 4: Run the provider tests again**

Run: `go test ./pkg/providers -run 'TestCreateProviderFromConfig_CodexAppServer|TestCodexAppServerProvider_RunInteractiveTurn_ForwardsRequest' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/providers/types.go \
        pkg/providers/codex_app_server_provider.go \
        pkg/providers/codex_app_server_provider_test.go \
        pkg/providers/factory_provider.go \
        pkg/providers/factory_provider_test.go
git commit -m "feat(codex): add app-server provider route"
```

## Task 4: Prefer The Interactive Provider Path In The Agent Loop

**Files:**
- Modify: `pkg/agent/loop.go`
- Modify: `pkg/agent/loop_test.go`

- [ ] **Step 1: Write a focused failing agent-loop test**

```go
func TestAgentLoop_UsesInteractiveProviderAndStreamsChunks(t *testing.T) {
	cfg := minimalTestConfig(t)
	msgBus := bus.NewMessageBus()

	streamer := &recordingStreamer{}
	msgBus.SetStreamDelegate(recordingStreamDelegate{streamer: streamer})

	provider := &recordingInteractiveProvider{
		response: &providers.LLMResponse{Content: "final answer", FinishReason: "stop"},
		chunks:   []string{"final", "final answer"},
	}

	al := NewAgentLoop(cfg, msgBus, provider)
	agent, _ := al.GetRegistry().GetAgent("default")

	_, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "hello",
	}, agent, nil)
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if provider.chatCalled {
		t.Fatal("expected interactive provider path, got Chat()")
	}
	if len(streamer.updates) != 2 || streamer.finalized != "final answer" {
		t.Fatalf("streamer = %#v, want 2 updates + finalization", streamer)
	}
}
```

- [ ] **Step 2: Run the agent test to verify it fails**

Run: `go test ./pkg/agent -run 'TestAgentLoop_UsesInteractiveProviderAndStreamsChunks' -count=1`

Expected: FAIL because the loop still calls `provider.Chat(...)` directly and never touches the bus streamer

- [ ] **Step 3: Add the interactive-provider branch in the LLM call path**

```go
callLLM := func(messagesForCall []providers.Message, toolDefsForCall []providers.ToolDefinition) (*providers.LLMResponse, error) {
	providerCtx, providerCancel := context.WithCancel(turnCtx)
	ts.setProviderCancel(providerCancel)
	defer func() {
		providerCancel()
		ts.clearProviderCancel(providerCancel)
	}()

	if interactive, ok := activeProvider.(providers.InteractiveProvider); ok {
		var streamer bus.Streamer
		if al.bus != nil {
			if s, found := al.bus.GetStreamer(providerCtx, ts.channel, ts.chatID); found {
				streamer = s
			}
		}

		resp, err := interactive.RunInteractiveTurn(providerCtx, providers.InteractiveTurnRequest{
			SessionKey: ts.sessionKey,
			AgentID:    ts.agentID,
			Channel:    ts.channel,
			ChatID:     ts.chatID,
			Model:      llmModel,
			Messages:   messagesForCall,
			Tools:      toolDefsForCall,
			Options:    llmOpts,
			OnChunk: func(accumulated string) {
				if streamer != nil {
					_ = streamer.Update(providerCtx, accumulated)
				}
			},
		})
		if streamer != nil {
			if err != nil {
				streamer.Cancel(providerCtx)
			} else {
				_ = streamer.Finalize(providerCtx, resp.Content)
			}
		}
		return resp, err
	}

	return activeProvider.Chat(providerCtx, messagesForCall, toolDefsForCall, llmModel, llmOpts)
}
```

- [ ] **Step 4: Run the targeted agent and provider tests**

Run: `go test ./pkg/agent ./pkg/providers ./pkg/codexruntime -run 'TestAgentLoop_UsesInteractiveProviderAndStreamsChunks|TestCreateProviderFromConfig_CodexAppServer|TestRunner_ResumeFallsBackToStart' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/loop.go pkg/agent/loop_test.go
git commit -m "feat(agent): stream codex app-server turns through interactive providers"
```

## Task 5: Full Phase Verification

**Files:**
- No new files

- [ ] **Step 1: Run focused package tests**

Run: `go test ./pkg/codexruntime ./pkg/providers ./pkg/agent -count=1`

Expected: PASS

- [ ] **Step 2: Run the broader provider and agent safety net**

Run: `go test ./pkg/providers/... ./pkg/agent/... -count=1`

Expected: PASS

- [ ] **Step 3: Run formatting**

Run: `make fmt`

Expected: exit code `0`

- [ ] **Step 4: Run lint or targeted verification**

Run: `make check`

Expected: exit code `0` or a clearly scoped failure unrelated to the new Codex foundation work

- [ ] **Step 5: Commit the verification pass**

```bash
git add .
git commit -m "test: verify codex runtime foundation"
```

## Self-Review

Spec coverage for phase 1:

- persistent `stdio` client: covered in Task 2
- durable bindings: covered in Task 1 and Task 2
- final-assistant-only projection: covered in Task 1
- Codex behind PicoClaw provider abstraction: covered in Task 3
- agent-loop streaming hookup: covered in Task 4

Known intentional gaps for later plans:

- tool bridge
- approval bridge
- MCP allowlists
- per-thread commands and runtime settings
- config rewrite
- launcher/channel/provider deletions

