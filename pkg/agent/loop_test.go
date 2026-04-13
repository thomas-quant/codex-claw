package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/routing"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type fakeChannel struct{ id string }

func (f *fakeChannel) Name() string                    { return "fake" }
func (f *fakeChannel) Start(ctx context.Context) error { return nil }
func (f *fakeChannel) Stop(ctx context.Context) error  { return nil }
func (f *fakeChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	return nil, nil
}
func (f *fakeChannel) IsRunning() bool                            { return true }
func (f *fakeChannel) IsAllowed(string) bool                      { return true }
func (f *fakeChannel) IsAllowedSender(sender bus.SenderInfo) bool { return true }
func (f *fakeChannel) ReasoningChannelID() string                 { return f.id }

type fakeMediaChannel struct {
	fakeChannel
	sentMedia []bus.OutboundMediaMessage
}

func (f *fakeMediaChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) ([]string, error) {
	f.sentMedia = append(f.sentMedia, msg)
	return nil, nil
}

type fakeStreamingChannel struct {
	fakeChannel
	mu          sync.Mutex
	beginCalls  int
	updates     []string
	finalized   []string
	cancelCalls int
	sent        []bus.OutboundMessage
	streamer    *fakeStreamer
}

func (f *fakeStreamingChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sent = append(f.sent, msg)
	return nil, nil
}

func (f *fakeStreamingChannel) BeginStream(ctx context.Context, chatID string) (channels.Streamer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.beginCalls++
	f.streamer = &fakeStreamer{channel: f, done: make(chan struct{})}
	return f.streamer, nil
}

func (f *fakeStreamingChannel) snapshot() ([]bus.OutboundMessage, *fakeStreamer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bus.OutboundMessage(nil), f.sent...), f.streamer
}

type fakeStreamer struct {
	channel *fakeStreamingChannel
	done    chan struct{}
	once    sync.Once
}

func (s *fakeStreamer) Update(ctx context.Context, content string) error {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	s.channel.updates = append(s.channel.updates, content)
	return nil
}

func (s *fakeStreamer) Finalize(ctx context.Context, content string) error {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	s.channel.finalized = append(s.channel.finalized, content)
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *fakeStreamer) Cancel(ctx context.Context) {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	s.channel.cancelCalls++
	s.once.Do(func() { close(s.done) })
}

func (s *fakeStreamer) snapshot() (updates, finalized []string, canceled bool) {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	return append([]string(nil), s.channel.updates...), append([]string(nil), s.channel.finalized...), s.channel.cancelCalls > 0
}

func newStartedTestChannelManager(
	t *testing.T,
	msgBus *bus.MessageBus,
	store media.MediaStore,
	name string,
	ch channels.Channel,
) *channels.Manager {
	t.Helper()

	cm, err := channels.NewManager(&config.Config{}, msgBus, store)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	cm.RegisterChannel(name, ch)
	if err := cm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}
	t.Cleanup(func() {
		if err := cm.StopAll(context.Background()); err != nil {
			t.Fatalf("StopAll() error = %v", err)
		}
	})
	return cm
}

type recordingProvider struct {
	lastMessages []providers.Message
}

func (r *recordingProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	r.lastMessages = append([]providers.Message(nil), messages...)
	return &providers.LLMResponse{
		Content:   "Mock response",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (r *recordingProvider) GetDefaultModel() string {
	return "mock-model"
}

type interactiveLoopProvider struct {
	mu                sync.Mutex
	chatCalled        bool
	interactiveCalled bool
	callOrder         []string
	updates           []providers.InteractiveTurnRequest
	chunks            []string
	finalContent      string
	toolCall          providers.InteractiveToolCall
	toolResults       []providers.InteractiveToolResult
	turnErrors        []error
	models            []providers.InteractiveModelInfo
	status            providers.InteractiveThreadStatus
	statusErr         error
	setModelCalls     []string
	setThinkingCalls  []string
	compactCalls      int
	resetCalls        int
}

func (p *interactiveLoopProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	p.mu.Lock()
	p.chatCalled = true
	p.mu.Unlock()
	return &providers.LLMResponse{Content: "chat path should not be used"}, nil
}

func (p *interactiveLoopProvider) GetDefaultModel() string {
	return "test-model"
}

func (p *interactiveLoopProvider) RunInteractiveTurn(
	ctx context.Context,
	req providers.InteractiveTurnRequest,
) (*providers.LLMResponse, error) {
	p.mu.Lock()
	p.interactiveCalled = true
	p.callOrder = append(p.callOrder, "interactive")
	p.updates = append(p.updates, req)
	chunks := append([]string(nil), p.chunks...)
	finalContent := p.finalContent
	var turnErr error
	if len(p.turnErrors) > 0 {
		turnErr = p.turnErrors[0]
		p.turnErrors = p.turnErrors[1:]
	}
	p.mu.Unlock()

	if turnErr != nil {
		return nil, turnErr
	}

	for _, chunk := range chunks {
		if req.OnChunk != nil {
			req.OnChunk(chunk)
		}
	}
	if p.toolCall.Name != "" {
		if req.ExecuteTool == nil {
			return nil, fmt.Errorf("interactive tool executor was nil")
		}
		result, err := req.ExecuteTool(ctx, p.toolCall)
		if err != nil {
			return nil, err
		}
		p.mu.Lock()
		p.toolResults = append(p.toolResults, result)
		p.mu.Unlock()
	}

	return &providers.LLMResponse{
		Content:      finalContent,
		FinishReason: "stop",
	}, nil
}

func (p *interactiveLoopProvider) snapshot() (chatCalled, interactiveCalled bool, reqs []providers.InteractiveTurnRequest, results []providers.InteractiveToolResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.chatCalled, p.interactiveCalled, append([]providers.InteractiveTurnRequest(nil), p.updates...), append([]providers.InteractiveToolResult(nil), p.toolResults...)
}

func (p *interactiveLoopProvider) ListModels(context.Context) ([]providers.InteractiveModelInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]providers.InteractiveModelInfo(nil), p.models...), nil
}

func (p *interactiveLoopProvider) ReadThreadStatus(context.Context, providers.InteractiveThreadControlRequest) (providers.InteractiveThreadStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status, p.statusErr
}

func (p *interactiveLoopProvider) SetThreadModel(_ context.Context, _ providers.InteractiveThreadControlRequest, model string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old := p.status.Model
	p.status.Model = model
	p.setModelCalls = append(p.setModelCalls, model)
	return old, nil
}

func (p *interactiveLoopProvider) SetThreadThinking(_ context.Context, _ providers.InteractiveThreadControlRequest, thinking string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old := p.status.ThinkingMode
	p.status.ThinkingMode = thinking
	p.setThinkingCalls = append(p.setThinkingCalls, thinking)
	return old, nil
}

func (p *interactiveLoopProvider) ToggleThreadFast(context.Context, providers.InteractiveThreadControlRequest) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.FastEnabled = !p.status.FastEnabled
	return p.status.FastEnabled, nil
}

func (p *interactiveLoopProvider) CompactThread(context.Context, providers.InteractiveThreadControlRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callOrder = append(p.callOrder, "compact")
	p.compactCalls++
	return nil
}

func (p *interactiveLoopProvider) ResetThread(context.Context, providers.InteractiveThreadControlRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resetCalls++
	p.status.ThreadID = ""
	p.status.RecoveryState = ""
	return nil
}

func newTestAgentLoop(
	t *testing.T,
) (al *AgentLoop, cfg *config.Config, msgBus *bus.MessageBus, provider *mockProvider, cleanup func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	cfg = &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	msgBus = bus.NewMessageBus()
	provider = &mockProvider{}
	al = NewAgentLoop(cfg, msgBus, provider)
	return al, cfg, msgBus, provider, func() { os.RemoveAll(tmpDir) }
}

func TestAgentLoop_UsesInteractiveProviderAndStreamsChunks(t *testing.T) {
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
		chunks:       []string{"Hello", " there"},
		finalContent: "Hello there",
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	streamChannel := &fakeStreamingChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, media.NewFileMediaStore(), "telegram", streamChannel))

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- al.Run(runCtx)
	}()

	if err := msgBus.PublishInbound(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "say hi",
	}); err != nil {
		t.Fatalf("PublishInbound() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	var streamer *fakeStreamer
	for streamer == nil {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for streamer")
		default:
			_, streamer = streamChannel.snapshot()
			if streamer == nil {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	select {
	case <-streamer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for interactive stream finalize")
	}

	chatCalled, interactiveCalled, reqs, _ := provider.snapshot()
	if chatCalled {
		t.Fatal("expected Chat fallback path to remain unused")
	}
	if !interactiveCalled {
		t.Fatal("expected interactive provider path to be used")
	}
	if len(reqs) != 1 {
		t.Fatalf("interactive request count = %d, want %d", len(reqs), 1)
	}
	if reqs[0].SessionKey == "" || reqs[0].AgentID == "" {
		t.Fatalf("interactive request identity = %+v, want session and agent identity", reqs[0])
	}

	updates, finalized, canceled := streamer.snapshot()
	if canceled {
		t.Fatal("expected direct-answer stream to finalize, not cancel")
	}
	if !slices.Equal(updates, []string{"Hello", "Hello there"}) {
		t.Fatalf("stream updates = %v, want %v", updates, []string{"Hello", "Hello there"})
	}
	if !slices.Equal(finalized, []string{"Hello there"}) {
		t.Fatalf("stream finalized = %v, want %v", finalized, []string{"Hello there"})
	}

	time.Sleep(100 * time.Millisecond)
	sent, _ := streamChannel.snapshot()
	if len(sent) != 0 {
		t.Fatalf("expected final outbound send to be suppressed after stream finalize, got %+v", sent)
	}

	runCancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run() to exit")
	}
}

type fakeInteractiveTool struct{}

func (fakeInteractiveTool) Name() string { return "lookup_weather" }

func (fakeInteractiveTool) Description() string { return "Looks up weather" }

func (fakeInteractiveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string"},
		},
	}
}

func (fakeInteractiveTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	return tools.NewToolResult(fmt.Sprintf("weather for %v: sunny", args["city"]))
}

type loopAsyncFollowUpTool struct{}

func (loopAsyncFollowUpTool) Name() string { return "async_follow_up" }

func (loopAsyncFollowUpTool) Description() string { return "Publishes a follow-up asynchronously" }

func (loopAsyncFollowUpTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (loopAsyncFollowUpTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	return tools.AsyncResult("async started")
}

func (loopAsyncFollowUpTool) ExecuteAsync(
	ctx context.Context,
	args map[string]any,
	cb tools.AsyncCallback,
) *tools.ToolResult {
	if cb != nil {
		cb(ctx, tools.UserResult("async follow-up ready"))
	}
	return tools.AsyncResult("async started")
}

type toolDecisionHook struct {
	beforeDenied   map[string]string
	approvalDenied map[string]string
	beforeRespond  map[string]*tools.ToolResult
	afterModified  map[string]*tools.ToolResult
}

func (h *toolDecisionHook) BeforeTool(
	ctx context.Context,
	call *ToolCallHookRequest,
) (*ToolCallHookRequest, HookDecision, error) {
	if reason, ok := h.beforeDenied[call.Tool]; ok {
		return call.Clone(), HookDecision{Action: HookActionDenyTool, Reason: reason}, nil
	}
	if result, ok := h.beforeRespond[call.Tool]; ok {
		next := call.Clone()
		next.HookResult = cloneToolResult(result)
		return next, HookDecision{Action: HookActionRespond}, nil
	}
	return call.Clone(), HookDecision{Action: HookActionContinue}, nil
}

func (h *toolDecisionHook) AfterTool(
	ctx context.Context,
	result *ToolResultHookResponse,
) (*ToolResultHookResponse, HookDecision, error) {
	if modifiedResult, ok := h.afterModified[result.Tool]; ok {
		next := result.Clone()
		next.Result = cloneToolResult(modifiedResult)
		return next, HookDecision{Action: HookActionModify}, nil
	}
	return result.Clone(), HookDecision{Action: HookActionContinue}, nil
}

func (h *toolDecisionHook) ApproveTool(
	ctx context.Context,
	req *ToolApprovalRequest,
) (ApprovalDecision, error) {
	if reason, ok := h.approvalDenied[req.Tool]; ok {
		return ApprovalDecision{Approved: false, Reason: reason}, nil
	}
	return ApprovalDecision{Approved: true}, nil
}

func TestAgentLoop_InteractiveProviderToolCallbackUsesAgentToolExecution(t *testing.T) {
	t.Run("successful tool execution", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID: "call-1",
				Name:   "lookup_weather",
				Arguments: map[string]any{
					"city": "London",
				},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		streamChannel := &fakeStreamingChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
		al.SetChannelManager(newStartedTestChannelManager(t, msgBus, media.NewFileMediaStore(), "telegram", streamChannel))

		runCtx, runCancel := context.WithCancel(context.Background())
		defer runCancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- al.Run(runCtx)
		}()

		if err := msgBus.PublishInbound(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		}); err != nil {
			t.Fatalf("PublishInbound() error = %v", err)
		}

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			_, interactiveCalled, _, results := provider.snapshot()
			if interactiveCalled && len(results) == 1 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		runCancel()
		select {
		case err := <-runDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop")
		}

		chatCalled, interactiveCalled, reqs, results := provider.snapshot()
		if chatCalled {
			t.Fatal("expected Chat() path to remain unused")
		}
		if !interactiveCalled {
			t.Fatal("expected interactive path to be used")
		}
		if len(reqs) != 1 {
			t.Fatalf("interactive requests = %d, want 1", len(reqs))
		}
		if reqs[0].ExecuteTool == nil {
			t.Fatal("interactive request ExecuteTool callback was nil")
		}
		if len(results) != 1 {
			t.Fatalf("tool callback results = %d, want 1", len(results))
		}
		if !results[0].Success {
			t.Fatalf("tool callback success = %v, want true", results[0].Success)
		}
		if len(results[0].ContentItems) != 1 || !strings.Contains(results[0].ContentItems[0].Text, "London") {
			t.Fatalf("tool callback result = %#v, want weather text for London", results[0])
		}

		mainAgent, ok := al.GetRegistry().GetAgent("main")
		if !ok {
			t.Fatal("main agent not found")
		}
		history := mainAgent.Sessions.GetHistory(reqs[0].SessionKey)
		if len(history) == 0 {
			t.Fatal("expected session history to contain interactive tool messages")
		}
		foundToolMessage := false
		for _, msg := range history {
			if msg.Role == "tool" && strings.Contains(msg.Content, "London") {
				foundToolMessage = true
				break
			}
		}
		if !foundToolMessage {
			t.Fatalf("session history = %#v, want persisted tool result", history)
		}
	})

	t.Run("hook denial", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-denied",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		if err := al.MountHook(NamedHook("deny-before", &toolDecisionHook{
			beforeDenied: map[string]string{"lookup_weather": "blocked by before hook"},
		})); err != nil {
			t.Fatalf("MountHook() error = %v", err)
		}

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, reqs, results := provider.snapshot()
		if len(reqs) != 1 || len(results) != 1 {
			t.Fatalf("interactive reqs/results = %d/%d, want 1/1", len(reqs), len(results))
		}
		if results[0].Success {
			t.Fatalf("tool callback success = %v, want false", results[0].Success)
		}
		if len(results[0].ContentItems) != 1 {
			t.Fatalf("tool callback content items = %d, want 1", len(results[0].ContentItems))
		}
		want := "Tool execution denied by hook: blocked by before hook"
		if results[0].ContentItems[0].Text != want {
			t.Fatalf("tool callback text = %q, want %q", results[0].ContentItems[0].Text, want)
		}

		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(reqs[0].SessionKey)
		foundDenied := false
		for _, msg := range history {
			if msg.Role == "tool" && msg.Content == want {
				foundDenied = true
				break
			}
		}
		if !foundDenied {
			t.Fatalf("session history = %#v, want denied tool message", history)
		}
	})

	t.Run("approval denial", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-denied",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		if err := al.MountHook(NamedHook("deny-approval", &toolDecisionHook{
			approvalDenied: map[string]string{"lookup_weather": "approval blocked"},
		})); err != nil {
			t.Fatalf("MountHook() error = %v", err)
		}

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, reqs, results := provider.snapshot()
		if len(reqs) != 1 || len(results) != 1 {
			t.Fatalf("interactive reqs/results = %d/%d, want 1/1", len(reqs), len(results))
		}
		if results[0].Success {
			t.Fatalf("tool callback success = %v, want false", results[0].Success)
		}
		want := "Tool execution denied by approval hook: approval blocked"
		if len(results[0].ContentItems) != 1 || results[0].ContentItems[0].Text != want {
			t.Fatalf("tool callback result = %#v, want %q", results[0], want)
		}

		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(reqs[0].SessionKey)
		foundDenied := false
		for _, msg := range history {
			if msg.Role == "tool" && msg.Content == want {
				foundDenied = true
				break
			}
		}
		if !foundDenied {
			t.Fatalf("session history = %#v, want approval denial message", history)
		}
	})

	t.Run("before hook respond result", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-respond",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		if err := al.MountHook(NamedHook("respond-before", &toolDecisionHook{
			beforeRespond: map[string]*tools.ToolResult{
				"lookup_weather": tools.NewToolResult("Hook handled weather for London."),
			},
		})); err != nil {
			t.Fatalf("MountHook() error = %v", err)
		}

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, reqs, results := provider.snapshot()
		if len(reqs) != 1 || len(results) != 1 {
			t.Fatalf("interactive reqs/results = %d/%d, want 1/1", len(reqs), len(results))
		}
		if !results[0].Success {
			t.Fatalf("tool callback success = %v, want true", results[0].Success)
		}
		if len(results[0].ContentItems) != 1 || results[0].ContentItems[0].Text != "Hook handled weather for London." {
			t.Fatalf("tool callback result = %#v, want hook response", results[0])
		}

		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(reqs[0].SessionKey)
		foundResponded := false
		for _, msg := range history {
			if msg.Role == "tool" && msg.Content == "Hook handled weather for London." {
				foundResponded = true
				break
			}
		}
		if !foundResponded {
			t.Fatalf("session history = %#v, want responded tool message", history)
		}
	})

	t.Run("after hook modify result", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-after-modify",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		if err := al.MountHook(NamedHook("modify-after", &toolDecisionHook{
			afterModified: map[string]*tools.ToolResult{
				"lookup_weather": tools.NewToolResult("After hook rewrote the weather result."),
			},
		})); err != nil {
			t.Fatalf("MountHook() error = %v", err)
		}

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, reqs, results := provider.snapshot()
		if len(reqs) != 1 || len(results) != 1 {
			t.Fatalf("interactive reqs/results = %d/%d, want 1/1", len(reqs), len(results))
		}
		if !results[0].Success {
			t.Fatalf("tool callback success = %v, want true", results[0].Success)
		}
		if len(results[0].ContentItems) != 1 || results[0].ContentItems[0].Text != "After hook rewrote the weather result." {
			t.Fatalf("tool callback result = %#v, want modified response", results[0])
		}

		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(reqs[0].SessionKey)
		foundModified := false
		for _, msg := range history {
			if msg.Role == "tool" && msg.Content == "After hook rewrote the weather result." {
				foundModified = true
				break
			}
		}
		if !foundModified {
			t.Fatalf("session history = %#v, want modified tool message", history)
		}
	})

	t.Run("async follow-up publication", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-async",
				Name:      "async_follow_up",
				Arguments: map[string]any{},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(loopAsyncFollowUpTool{})

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use async tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, _, results := provider.snapshot()
		if len(results) != 1 || !results[0].Success {
			t.Fatalf("tool callback result = %#v, want one successful result", results)
		}

		select {
		case outbound := <-msgBus.OutboundChan():
			if outbound.Content != "async follow-up ready" {
				t.Fatalf("async outbound content = %q, want %q", outbound.Content, "async follow-up ready")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for async outbound publication")
		}

		select {
		case inbound := <-msgBus.InboundChan():
			if inbound.Channel != "system" {
				t.Fatalf("async inbound channel = %q, want system", inbound.Channel)
			}
			if inbound.SenderID != "async:async_follow_up" {
				t.Fatalf("async inbound sender = %q, want %q", inbound.SenderID, "async:async_follow_up")
			}
			if inbound.Content != "async follow-up ready" {
				t.Fatalf("async inbound content = %q, want %q", inbound.Content, "async follow-up ready")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for async inbound publication")
		}
	})

	t.Run("handled media response", func(t *testing.T) {
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
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-media",
				Name:      "handled_media_tool",
				Arguments: map[string]any{},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)

		store := media.NewFileMediaStore()
		al.SetMediaStore(store)
		telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
		al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

		imagePath := filepath.Join(tmpDir, "screen.png")
		if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
			t.Fatalf("WriteFile(imagePath) error = %v", err)
		}
		al.RegisterTool(&handledMediaTool{store: store, path: imagePath})

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "send a screenshot",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, reqs, results := provider.snapshot()
		if len(reqs) != 1 || len(results) != 1 {
			t.Fatalf("interactive reqs/results = %d/%d, want 1/1", len(reqs), len(results))
		}
		if len(results[0].ContentItems) != 1 || !strings.Contains(results[0].ContentItems[0].Text, "already been delivered") {
			t.Fatalf("tool callback result = %#v, want handled-delivery note", results[0])
		}
		if len(telegramChannel.sentMedia) != 1 {
			t.Fatalf("sent media count = %d, want 1", len(telegramChannel.sentMedia))
		}

		select {
		case extra := <-msgBus.OutboundMediaChan():
			t.Fatalf("expected handled media to bypass async queue, got %+v", extra)
		default:
		}

		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(reqs[0].SessionKey)
		foundHandled := false
		for _, msg := range history {
			if msg.Role == "tool" && strings.Contains(msg.Content, "already been delivered") {
				foundHandled = true
				break
			}
		}
		if !foundHandled {
			t.Fatalf("session history = %#v, want handled tool message", history)
		}
	})

	t.Run("artifact media is forwarded to follow-up tools", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := config.DefaultConfig()
		cfg.Agents.Defaults.Workspace = tmpDir
		cfg.Agents.Defaults.ModelName = "test-model"
		cfg.Agents.Defaults.MaxTokens = 4096
		cfg.Agents.Defaults.MaxToolIterations = 10

		msgBus := bus.NewMessageBus()
		provider := &interactiveLoopProvider{
			finalContent: "done",
			toolCall: providers.InteractiveToolCall{
				CallID:    "call-artifact-media",
				Name:      "media_artifact_tool",
				Arguments: map[string]any{},
			},
		}
		al := NewAgentLoop(cfg, msgBus, provider)

		store := media.NewFileMediaStore()
		al.SetMediaStore(store)
		telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
		al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

		mediaDir := media.TempDir()
		if err := os.MkdirAll(mediaDir, 0o700); err != nil {
			t.Fatalf("MkdirAll(mediaDir) error = %v", err)
		}
		imagePath := filepath.Join(mediaDir, "interactive-artifact-screen.png")
		if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
			t.Fatalf("WriteFile(imagePath) error = %v", err)
		}

		al.RegisterTool(&mediaArtifactTool{
			store: store,
			path:  imagePath,
		})

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "make an artifact",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		_, _, reqs, results := provider.snapshot()
		if len(reqs) != 1 || len(results) != 1 {
			t.Fatalf("interactive reqs/results = %d/%d, want 1/1", len(reqs), len(results))
		}
		if !results[0].Success {
			t.Fatalf("tool callback success = %v, want true", results[0].Success)
		}
		if len(results[0].ContentItems) != 1 || !strings.Contains(results[0].ContentItems[0].Text, "[file:") {
			t.Fatalf("tool callback result = %#v, want artifact-tagged content", results[0])
		}

		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(reqs[0].SessionKey)
		foundArtifactMedia := false
		for _, msg := range history {
			if msg.Role == "tool" && len(msg.Media) == 1 && strings.Contains(msg.Content, "[file:") {
				foundArtifactMedia = true
				break
			}
		}
		if !foundArtifactMedia {
			t.Fatalf("session history = %#v, want tool message with artifact media and tag", history)
		}
	})
}

func TestAgentLoop_BuildCommandsRuntime_UsesInteractiveRuntimeController(t *testing.T) {
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
		models: []providers.InteractiveModelInfo{
			{ID: "gpt-5.4", Provider: "codex"},
			{ID: "gpt-5.4-mini", Provider: "codex"},
		},
		status: providers.InteractiveThreadStatus{
			ThreadID:          "thr_123",
			Model:             "gpt-5.4",
			Provider:          "codex",
			ThinkingMode:      "medium",
			FastEnabled:       false,
			LastUserMessageAt: time.Date(2026, time.April, 13, 10, 15, 0, 0, time.UTC),
			LastCompactionAt:  time.Date(2026, time.April, 13, 10, 45, 0, 0, time.UTC),
			ForceFreshThread:  true,
			RecoveryState:     "resumed",
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	agent, ok := al.GetRegistry().GetAgent("main")
	if !ok {
		t.Fatal("main agent not found")
	}

	rt := al.buildCommandsRuntime(agent, &processOptions{
		SessionKey: "session-1",
		Channel:    "telegram",
		ChatID:     "chat-1",
	})
	if rt == nil {
		t.Fatal("buildCommandsRuntime() returned nil")
	}

	models := rt.ListModels()
	if len(models) != 2 || models[0].Name != "gpt-5.4" || models[1].Name != "gpt-5.4-mini" {
		t.Fatalf("ListModels() = %#v, want codex models", models)
	}

	status, ok := rt.ReadStatus()
	if !ok {
		t.Fatal("ReadStatus() returned unavailable status")
	}
	if status.ThreadID != "thr_123" || status.Model != "gpt-5.4" || status.ThinkingMode != "medium" {
		t.Fatalf("ReadStatus() = %#v, want thread/model/thinking state", status)
	}
	if !status.LastUserMessageAt.Equal(time.Date(2026, time.April, 13, 10, 15, 0, 0, time.UTC)) {
		t.Fatalf("ReadStatus() last_user_message_at = %v, want %v", status.LastUserMessageAt, time.Date(2026, time.April, 13, 10, 15, 0, 0, time.UTC))
	}
	if !status.LastCompactionAt.Equal(time.Date(2026, time.April, 13, 10, 45, 0, 0, time.UTC)) {
		t.Fatalf("ReadStatus() last_compaction_at = %v, want %v", status.LastCompactionAt, time.Date(2026, time.April, 13, 10, 45, 0, 0, time.UTC))
	}
	if !status.ForceFreshThread {
		t.Fatal("ReadStatus() force_fresh_thread = false, want true")
	}

	oldModel, err := rt.SetModel("gpt-5.4-mini")
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if oldModel != "gpt-5.4" {
		t.Fatalf("SetModel() old = %q, want %q", oldModel, "gpt-5.4")
	}

	oldThinking, err := rt.SetThinking("high")
	if err != nil {
		t.Fatalf("SetThinking() error = %v", err)
	}
	if oldThinking != "medium" {
		t.Fatalf("SetThinking() old = %q, want %q", oldThinking, "medium")
	}

	fastEnabled, err := rt.ToggleFast()
	if err != nil {
		t.Fatalf("ToggleFast() error = %v", err)
	}
	if !fastEnabled {
		t.Fatal("ToggleFast() = false, want true")
	}

	if err := rt.CompactThread(); err != nil {
		t.Fatalf("CompactThread() error = %v", err)
	}
	if err := rt.ResetThread(); err != nil {
		t.Fatalf("ResetThread() error = %v", err)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.compactCalls != 1 {
		t.Fatalf("CompactThread() calls = %d, want 1", provider.compactCalls)
	}
	if provider.resetCalls != 1 {
		t.Fatalf("ResetThread() calls = %d, want 1", provider.resetCalls)
	}
	if len(provider.setModelCalls) != 1 || provider.setModelCalls[0] != "gpt-5.4-mini" {
		t.Fatalf("SetModel() calls = %v, want %v", provider.setModelCalls, []string{"gpt-5.4-mini"})
	}
	if len(provider.setThinkingCalls) != 1 || provider.setThinkingCalls[0] != "high" {
		t.Fatalf("SetThinking() calls = %v, want %v", provider.setThinkingCalls, []string{"high"})
	}
	if provider.status.ThreadID != "" {
		t.Fatalf("ResetThread() left thread id = %q, want empty", provider.status.ThreadID)
	}
	if provider.status.Model != "gpt-5.4-mini" || provider.status.ThinkingMode != "high" || !provider.status.FastEnabled {
		t.Fatalf("provider status after callbacks = %+v, want persisted model/thinking/fast", provider.status)
	}
}

func TestAgentLoop_InteractiveProviderPassesThreadRuntimeControl(t *testing.T) {
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
			ThreadID:     "thr_123",
			Model:        "gpt-5.4-mini",
			ThinkingMode: "high",
			FastEnabled:  true,
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "say hi",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "done" {
		t.Fatalf("response = %q, want %q", response, "done")
	}

	_, interactiveCalled, reqs, _ := provider.snapshot()
	if !interactiveCalled || len(reqs) != 1 {
		t.Fatalf("interactive provider requests = %d, want 1", len(reqs))
	}
	if reqs[0].Model != "gpt-5.4-mini" {
		t.Fatalf("request model = %q, want %q", reqs[0].Model, "gpt-5.4-mini")
	}
	if reqs[0].Control.ThinkingMode != "high" {
		t.Fatalf("request thinking = %q, want %q", reqs[0].Control.ThinkingMode, "high")
	}
	if !reqs[0].Control.FastEnabled {
		t.Fatal("request fast_enabled = false, want true")
	}
	if !reqs[0].Control.FastEnabledSet {
		t.Fatal("request fast_enabled_set = false, want true")
	}
	if reqs[0].Control.LastUserMessageAt == "" {
		t.Fatal("request last_user_message_at is empty")
	}
	if reqs[0].Recovery != (providers.InteractiveRecoveryRequest{AllowServerRestart: true, AllowResume: true}) {
		t.Fatalf("request recovery = %#v, want restart+resume enabled", reqs[0].Recovery)
	}
}

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
		finalContent: "first reply",
		status: providers.InteractiveThreadStatus{
			ThreadID: "thr_123",
			Model:    "gpt-5.4",
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	if _, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "first question",
	}); err != nil {
		t.Fatalf("first processMessage() error = %v", err)
	}

	provider.mu.Lock()
	provider.status.ThreadID = ""
	provider.finalContent = "second reply"
	provider.mu.Unlock()

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "second question",
	})
	if err != nil {
		t.Fatalf("second processMessage() error = %v", err)
	}
	if response != "second reply" {
		t.Fatalf("response = %q, want %q", response, "second reply")
	}

	_, interactiveCalled, reqs, _ := provider.snapshot()
	if !interactiveCalled || len(reqs) != 2 {
		t.Fatalf("interactive provider requests = %d, want 2", len(reqs))
	}
	if reqs[0].BootstrapInput != "" {
		t.Fatalf("first request bootstrap_input = %q, want empty for existing thread", reqs[0].BootstrapInput)
	}

	wantBootstrap := buildInteractiveBootstrapInput(reqs[1].Messages, 3)
	if wantBootstrap == "" {
		t.Fatal("want bootstrap_input to be populated from history")
	}
	if reqs[1].BootstrapInput != wantBootstrap {
		t.Fatalf("bootstrap_input = %q, want %q", reqs[1].BootstrapInput, wantBootstrap)
	}
}

func TestAgentLoop_InteractiveProviderForcesFreshThreadAfterInactivity(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			Codex: config.CodexRuntimeConfig{
				AutoCompactThresholdPercent: 100,
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
			ThreadID:          "thr_123",
			Model:             "gpt-5.4",
			LastUserMessageAt: time.Now().UTC().Add(-(interactiveThreadInactivityLimit + time.Minute)),
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "wake the thread back up",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "done" {
		t.Fatalf("response = %q, want %q", response, "done")
	}

	_, interactiveCalled, reqs, _ := provider.snapshot()
	if !interactiveCalled || len(reqs) != 1 {
		t.Fatalf("interactive provider requests = %d, want 1", len(reqs))
	}
	if !reqs[0].Control.ForceFreshThread {
		t.Fatal("request force_fresh_thread = false, want true after inactivity")
	}

	wantBootstrap := buildInteractiveBootstrapInput(reqs[0].Messages, 3)
	if reqs[0].BootstrapInput != wantBootstrap {
		t.Fatalf("bootstrap_input = %q, want %q", reqs[0].BootstrapInput, wantBootstrap)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.compactCalls != 0 {
		t.Fatalf("CompactThread() calls = %d, want 0 when force_fresh_thread is true", provider.compactCalls)
	}
}

func TestAgentLoop_InteractiveProviderHonorsRuntimeForceFreshSignal(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			Codex: config.CodexRuntimeConfig{
				AutoCompactThresholdPercent: 100,
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
			ThreadID:          "thr_123",
			Model:             "gpt-5.4",
			LastUserMessageAt: time.Now().UTC().Add(-time.Hour),
			ForceFreshThread:  true,
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "runtime says start fresh",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "done" {
		t.Fatalf("response = %q, want %q", response, "done")
	}

	_, interactiveCalled, reqs, _ := provider.snapshot()
	if !interactiveCalled || len(reqs) != 1 {
		t.Fatalf("interactive provider requests = %d, want 1", len(reqs))
	}
	if !reqs[0].Control.ForceFreshThread {
		t.Fatal("request force_fresh_thread = false, want true from runtime status")
	}
	wantBootstrap := buildInteractiveBootstrapInput(reqs[0].Messages, 3)
	if reqs[0].BootstrapInput != wantBootstrap {
		t.Fatalf("bootstrap_input = %q, want %q", reqs[0].BootstrapInput, wantBootstrap)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.compactCalls != 0 {
		t.Fatalf("CompactThread() calls = %d, want 0 when runtime requests force fresh", provider.compactCalls)
	}
}

func TestAgentLoop_InteractiveProviderCompactsExistingThreadBeforeTurn(t *testing.T) {
	tmpDir := t.TempDir()
	const threshold = 20
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			Codex: config.CodexRuntimeConfig{
				AutoCompactThresholdPercent: threshold,
			},
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         1800,
				ContextWindow:     2400,
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
			LastUserMessageAt: time.Now().UTC().Add(-time.Hour),
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	agent, ok := al.GetRegistry().GetAgent("main")
	if !ok {
		t.Fatal("main agent not found")
	}

	content := "compact me"
	var remaining int
	for i := 0; i < 64; i++ {
		messages := agent.ContextBuilder.BuildMessages(
			nil,
			"",
			content,
			nil,
			"telegram",
			"chat-1",
			"user-1",
			"",
			activeSkillNames(agent, processOptions{})...,
		)
		remaining = remainingContextPercent(
			agent.ContextWindow,
			messages,
			agent.Tools.ToProviderDefs(),
			agent.MaxTokens,
		)
		if !isOverContextBudget(agent.ContextWindow, messages, agent.Tools.ToProviderDefs(), agent.MaxTokens) &&
			remaining <= threshold {
			break
		}
		content += " compact me"
	}
	if remaining > threshold {
		t.Fatalf("remaining context = %d%%, want <= %d%% before exercising proactive compaction", remaining, threshold)
	}

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  content,
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "done" {
		t.Fatalf("response = %q, want %q", response, "done")
	}

	_, interactiveCalled, reqs, _ := provider.snapshot()
	if !interactiveCalled || len(reqs) != 1 {
		t.Fatalf("interactive provider requests = %d, want 1", len(reqs))
	}
	if reqs[0].Control.ForceFreshThread {
		t.Fatal("request force_fresh_thread = true, want false for recent activity")
	}
	if reqs[0].BootstrapInput != "" {
		t.Fatalf("bootstrap_input = %q, want empty for reused thread", reqs[0].BootstrapInput)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.compactCalls != 1 {
		t.Fatalf("CompactThread() calls = %d, want 1", provider.compactCalls)
	}
	if !slices.Equal(provider.callOrder, []string{"compact", "interactive"}) {
		t.Fatalf("call order = %v, want compact before interactive", provider.callOrder)
	}
}

func TestAgentLoop_InteractiveProviderFallsBackToDeepSeekOnUsageExhaustion(t *testing.T) {
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
		turnErrors: []error{errors.New("usage limit reached")},
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.GetRegistry().GetDefaultAgent()
	fallback := &countingMockProvider{response: "deepseek fallback"}
	agent.DeepSeekFallback = fallback
	agent.DeepSeekFallbackModel = "deepseek-chat"

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "deepseek fallback" {
		t.Fatalf("response = %q, want %q", response, "deepseek fallback")
	}

	chatCalled, interactiveCalled, reqs, _ := provider.snapshot()
	if chatCalled {
		t.Fatal("expected Chat fallback path to remain unused")
	}
	if !interactiveCalled {
		t.Fatal("expected interactive provider path to be used")
	}
	if len(reqs) != 1 {
		t.Fatalf("interactive request count = %d, want 1", len(reqs))
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestAgentLoop_InteractiveProviderFallsBackToDeepSeekOnStartupFailure(t *testing.T) {
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
		turnErrors: []error{errors.New("codexruntime: start stdio transport: executable file not found")},
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.GetRegistry().GetDefaultAgent()
	fallback := &countingMockProvider{response: "deepseek fallback"}
	agent.DeepSeekFallback = fallback
	agent.DeepSeekFallbackModel = "deepseek-chat"

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "deepseek fallback" {
		t.Fatalf("response = %q, want %q", response, "deepseek fallback")
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestAgentLoop_InteractiveProviderDoesNotAutoFallbackOnOrdinaryError(t *testing.T) {
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
		turnErrors: []error{errors.New("tool bridge exploded")},
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.GetRegistry().GetDefaultAgent()
	fallback := &countingMockProvider{response: "deepseek fallback"}
	agent.DeepSeekFallback = fallback
	agent.DeepSeekFallbackModel = "deepseek-chat"

	_, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Content:  "hello",
	})
	if err == nil {
		t.Fatal("processMessage() error = nil, want ordinary codex error")
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestProcessMessage_ContextOverflow_InteractiveProviderUsesNativeCompaction(t *testing.T) {
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
		finalContent: "Recovered from overflow",
		turnErrors:   []error{errors.New("context_window_exceeded")},
		status: providers.InteractiveThreadStatus{
			ThreadID: "thr_123",
			Model:    "gpt-5.4",
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "test",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "trigger recovery",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Recovered from overflow" {
		t.Fatalf("response = %q, want %q", response, "Recovered from overflow")
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.compactCalls != 1 {
		t.Fatalf("CompactThread() calls = %d, want 1", provider.compactCalls)
	}
	if len(provider.updates) != 2 {
		t.Fatalf("RunInteractiveTurn() calls = %d, want 2", len(provider.updates))
	}
}

func TestProcessMessage_IncludesCurrentSenderInDynamicContext(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "discord",
		SenderID: "discord:123",
		Sender: bus.SenderInfo{
			DisplayName: "Alice",
		},
		ChatID:  "group-1",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Mock response" {
		t.Fatalf("processMessage() response = %q, want %q", response, "Mock response")
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("provider did not receive any messages")
	}

	systemPrompt := provider.lastMessages[0].Content
	wantSender := "## Current Sender\nCurrent sender: Alice (ID: discord:123)"
	if !strings.Contains(systemPrompt, wantSender) {
		t.Fatalf("system prompt missing sender context %q:\n%s", wantSender, systemPrompt)
	}

	lastMessage := provider.lastMessages[len(provider.lastMessages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "hello" {
		t.Fatalf("last provider message = %+v, want unchanged user message", lastMessage)
	}
}

func TestProcessMessage_UseCommandLoadsRequestedSkill(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills", "shell")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("# shell\n\nPrefer concise shell commands and explain them briefly."),
		0o644,
	); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "/use shell explain how to list files",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Mock response" {
		t.Fatalf("processMessage() response = %q, want %q", response, "Mock response")
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("provider did not receive any messages")
	}

	systemPrompt := provider.lastMessages[0].Content
	if !strings.Contains(systemPrompt, "# Active Skills") {
		t.Fatalf("system prompt missing active skills section:\n%s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "### Skill: shell") {
		t.Fatalf("system prompt missing requested skill content:\n%s", systemPrompt)
	}

	lastMessage := provider.lastMessages[len(provider.lastMessages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "explain how to list files" {
		t.Fatalf("last provider message = %+v, want rewritten user message", lastMessage)
	}
}

func TestHandleCommand_UseCommandRejectsUnknownSkill(t *testing.T) {
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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.GetRegistry().GetDefaultAgent()

	opts := processOptions{}
	reply, handled := al.handleCommand(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "/use missing explain how to list files",
	}, agent, &opts)
	if !handled {
		t.Fatal("expected /use with unknown skill to be handled")
	}
	if !strings.Contains(reply, "Unknown skill: missing") {
		t.Fatalf("reply = %q, want unknown skill error", reply)
	}
}

func TestProcessMessage_UseCommandArmsSkillForNextMessage(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills", "shell")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("# shell\n\nPrefer concise shell commands and explain them briefly."),
		0o644,
	); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "/use shell",
	})
	if err != nil {
		t.Fatalf("processMessage() arm error = %v", err)
	}
	if !strings.Contains(response, `Skill "shell" is armed for your next message.`) {
		t.Fatalf("arm response = %q, want armed confirmation", response)
	}

	response, err = al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "explain how to list files",
	})
	if err != nil {
		t.Fatalf("processMessage() follow-up error = %v", err)
	}
	if response != "Mock response" {
		t.Fatalf("follow-up response = %q, want %q", response, "Mock response")
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("provider did not receive any messages")
	}

	systemPrompt := provider.lastMessages[0].Content
	if !strings.Contains(systemPrompt, "### Skill: shell") {
		t.Fatalf("system prompt missing pending skill content:\n%s", systemPrompt)
	}
	lastMessage := provider.lastMessages[len(provider.lastMessages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "explain how to list files" {
		t.Fatalf("last provider message = %+v, want unchanged follow-up user message", lastMessage)
	}
}

func TestApplyExplicitSkillCommand_ArmsSkillForNextMessage(t *testing.T) {
	al, cfg, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news", "SKILL.md"),
		[]byte("# Finance News\n\nUse web tools for current finance updates.\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	opts := &processOptions{SessionKey: "agent:main:test"}
	matched, handled, reply := al.applyExplicitSkillCommand("/use finance-news", agent, opts)
	if !matched {
		t.Fatal("expected /use command to match")
	}
	if !handled {
		t.Fatal("expected /use without inline message to be handled immediately")
	}
	if !strings.Contains(reply, `Skill "finance-news" is armed for your next message`) {
		t.Fatalf("unexpected reply: %q", reply)
	}

	pending := al.takePendingSkills(opts.SessionKey)
	if len(pending) != 1 || pending[0] != "finance-news" {
		t.Fatalf("pending skills = %#v, want [finance-news]", pending)
	}
}

func TestApplyExplicitSkillCommand_InlineMessageMutatesOptions(t *testing.T) {
	al, cfg, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news", "SKILL.md"),
		[]byte("# Finance News\n\nUse web tools for current finance updates.\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	opts := &processOptions{
		SessionKey:  "agent:main:test",
		UserMessage: "/use finance-news dammi le ultime news",
	}
	matched, handled, reply := al.applyExplicitSkillCommand(opts.UserMessage, agent, opts)
	if !matched {
		t.Fatal("expected /use command to match")
	}
	if handled {
		t.Fatal("expected /use with inline message to fall through into normal agent execution")
	}
	if reply != "" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if opts.UserMessage != "dammi le ultime news" {
		t.Fatalf("opts.UserMessage = %q, want %q", opts.UserMessage, "dammi le ultime news")
	}
	if len(opts.ForcedSkills) != 1 || opts.ForcedSkills[0] != "finance-news" {
		t.Fatalf("opts.ForcedSkills = %#v, want [finance-news]", opts.ForcedSkills)
	}
}

func TestRecordLastChannel(t *testing.T) {
	al, cfg, msgBus, provider, cleanup := newTestAgentLoop(t)
	defer cleanup()

	testChannel := "test-channel"
	if err := al.RecordLastChannel(testChannel); err != nil {
		t.Fatalf("RecordLastChannel failed: %v", err)
	}
	if got := al.state.GetLastChannel(); got != testChannel {
		t.Errorf("Expected channel '%s', got '%s'", testChannel, got)
	}
	al2 := NewAgentLoop(cfg, msgBus, provider)
	if got := al2.state.GetLastChannel(); got != testChannel {
		t.Errorf("Expected persistent channel '%s', got '%s'", testChannel, got)
	}
}

func TestRecordLastChatID(t *testing.T) {
	al, cfg, msgBus, provider, cleanup := newTestAgentLoop(t)
	defer cleanup()

	testChatID := "test-chat-id-123"
	if err := al.RecordLastChatID(testChatID); err != nil {
		t.Fatalf("RecordLastChatID failed: %v", err)
	}
	if got := al.state.GetLastChatID(); got != testChatID {
		t.Errorf("Expected chat ID '%s', got '%s'", testChatID, got)
	}
	al2 := NewAgentLoop(cfg, msgBus, provider)
	if got := al2.state.GetLastChatID(); got != testChatID {
		t.Errorf("Expected persistent chat ID '%s', got '%s'", testChatID, got)
	}
}

func TestNewAgentLoop_StateInitialized(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
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

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Verify state manager is initialized
	if al.state == nil {
		t.Error("Expected state manager to be initialized")
	}

	// Verify state directory was created
	stateDir := filepath.Join(tmpDir, "state")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("Expected state directory to exist")
	}
}

// TestToolRegistry_ToolRegistration verifies tools can be registered and retrieved
func TestToolRegistry_ToolRegistration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Register a custom tool
	customTool := &mockCustomTool{}
	al.RegisterTool(customTool)

	// Verify tool is registered by checking it doesn't panic on GetStartupInfo
	// (actual tool retrieval is tested in tools package tests)
	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]any)
	toolsList := toolsInfo["names"].([]string)

	// Check that our custom tool name is in the list
	found := slices.Contains(toolsList, "mock_custom")
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestToolContext_Updates verifies tool context helpers work correctly
func TestToolContext_Updates(t *testing.T) {
	ctx := tools.WithToolContext(context.Background(), "telegram", "chat-42")

	if got := tools.ToolChannel(ctx); got != "telegram" {
		t.Errorf("expected channel 'telegram', got %q", got)
	}
	if got := tools.ToolChatID(ctx); got != "chat-42" {
		t.Errorf("expected chatID 'chat-42', got %q", got)
	}

	// Empty context returns empty strings
	if got := tools.ToolChannel(context.Background()); got != "" {
		t.Errorf("expected empty channel from bare context, got %q", got)
	}

	inboundCtx := tools.WithToolInboundContext(
		context.Background(),
		"telegram",
		"chat-42",
		"msg-123",
		"msg-100",
	)
	if got := tools.ToolMessageID(inboundCtx); got != "msg-123" {
		t.Errorf("expected messageID 'msg-123', got %q", got)
	}
	if got := tools.ToolReplyToMessageID(inboundCtx); got != "msg-100" {
		t.Errorf("expected replyToMessageID 'msg-100', got %q", got)
	}
}

// TestToolRegistry_GetDefinitions verifies tool definitions can be retrieved
func TestToolRegistry_GetDefinitions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Register a test tool and verify it shows up in startup info
	testTool := &mockCustomTool{}
	al.RegisterTool(testTool)

	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]any)
	toolsList := toolsInfo["names"].([]string)

	// Check that our custom tool name is in the list
	found := slices.Contains(toolsList, "mock_custom")
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

func TestProcessMessage_MediaToolHandledSkipsFollowUpLLMAndFinalText(t *testing.T) {
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
	provider := &handledMediaProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	store := media.NewFileMediaStore()
	al.SetMediaStore(store)
	telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

	imagePath := filepath.Join(tmpDir, "screen.png")
	if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
		t.Fatalf("WriteFile(imagePath) error = %v", err)
	}

	al.RegisterTool(&handledMediaTool{
		store: store,
		path:  imagePath,
	})

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "" {
		t.Fatalf("expected no final response when media tool already handled delivery, got %q", response)
	}
	if provider.calls != 1 {
		t.Fatalf("expected exactly 1 LLM call, got %d", provider.calls)
	}
	if len(provider.toolCounts) != 1 {
		t.Fatalf("expected tool counts for 1 provider call, got %d", len(provider.toolCounts))
	}
	if provider.toolCounts[0] == 0 {
		t.Fatal("expected tools to be available on the first LLM call")
	}

	if len(telegramChannel.sentMedia) != 1 {
		t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
	}
	if telegramChannel.sentMedia[0].Channel != "telegram" || telegramChannel.sentMedia[0].ChatID != "chat1" {
		t.Fatalf("unexpected sent media target: %+v", telegramChannel.sentMedia[0])
	}
	if len(telegramChannel.sentMedia[0].Parts) != 1 {
		t.Fatalf("expected exactly 1 sent media part, got %d", len(telegramChannel.sentMedia[0].Parts))
	}

	select {
	case extra := <-msgBus.OutboundMediaChan():
		t.Fatalf("expected handled media to bypass async queue, got %+v", extra)
	default:
	}

	defaultAgent := al.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}
	route, _, err := al.resolveMessageRoute(bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("resolveMessageRoute() error = %v", err)
	}
	sessionKey := resolveScopeKey(route, "")
	history := defaultAgent.Sessions.GetHistory(sessionKey)
	if len(history) == 0 {
		t.Fatal("expected session history to be saved")
	}
	last := history[len(history)-1]
	if last.Role != "assistant" || last.Content != "Requested output delivered via tool attachment." {
		t.Fatalf("expected handled assistant summary in history, got %+v", last)
	}
}

func TestProcessMessage_HandledToolProcessesQueuedSteeringBeforeReturning(t *testing.T) {
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
	provider := &handledMediaWithSteeringProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	store := media.NewFileMediaStore()
	al.SetMediaStore(store)
	telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

	imagePath := filepath.Join(tmpDir, "screen-steering.png")
	if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
		t.Fatalf("WriteFile(imagePath) error = %v", err)
	}

	al.RegisterTool(&handledMediaWithSteeringTool{
		store: store,
		path:  imagePath,
		loop:  al,
	})

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Handled the queued steering message." {
		t.Fatalf("response = %q, want queued steering response", response)
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 LLM calls after queued steering, got %d", provider.calls)
	}
	if len(telegramChannel.sentMedia) != 1 {
		t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
	}
}

func TestProcessMessage_MediaArtifactCanBeForwardedBySendFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.Agents.Defaults.ModelName = "test-model"
	cfg.Agents.Defaults.MaxTokens = 4096
	cfg.Agents.Defaults.MaxToolIterations = 10

	msgBus := bus.NewMessageBus()
	provider := &artifactThenSendProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	store := media.NewFileMediaStore()
	al.SetMediaStore(store)
	telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

	mediaDir := media.TempDir()
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(mediaDir) error = %v", err)
	}
	imagePath := filepath.Join(mediaDir, "artifact-screen.png")
	if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
		t.Fatalf("WriteFile(imagePath) error = %v", err)
	}

	al.RegisterTool(&mediaArtifactTool{
		store: store,
		path:  imagePath,
	})

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "" {
		t.Fatalf("expected no final response after send_file handled delivery, got %q", response)
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 LLM calls (artifact + send_file), got %d", provider.calls)
	}

	if len(telegramChannel.sentMedia) != 1 {
		t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
	}
	if telegramChannel.sentMedia[0].Channel != "telegram" || telegramChannel.sentMedia[0].ChatID != "chat1" {
		t.Fatalf("unexpected sent media target: %+v", telegramChannel.sentMedia[0])
	}
	if len(telegramChannel.sentMedia[0].Parts) != 1 {
		t.Fatalf("expected exactly 1 sent media part, got %d", len(telegramChannel.sentMedia[0].Parts))
	}

	select {
	case extra := <-msgBus.OutboundMediaChan():
		t.Fatalf("expected synchronous send_file delivery to bypass async queue, got %+v", extra)
	default:
	}
}

// TestAgentLoop_GetStartupInfo verifies startup info contains tools
func TestAgentLoop_GetStartupInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.Agents.Defaults.ModelName = "test-model"
	cfg.Agents.Defaults.MaxTokens = 4096
	cfg.Agents.Defaults.MaxToolIterations = 10

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	info := al.GetStartupInfo()

	// Verify tools info exists
	toolsInfo, ok := info["tools"]
	if !ok {
		t.Fatal("Expected 'tools' key in startup info")
	}

	toolsMap, ok := toolsInfo.(map[string]any)
	if !ok {
		t.Fatal("Expected 'tools' to be a map")
	}

	count, ok := toolsMap["count"]
	if !ok {
		t.Fatal("Expected 'count' in tools info")
	}

	// Should have default tools registered
	if count.(int) == 0 {
		t.Error("Expected at least some tools to be registered")
	}
}

// TestAgentLoop_Stop verifies Stop() sets running to false
func TestAgentLoop_Stop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Note: running is only set to true when Run() is called
	// We can't test that without starting the event loop
	// Instead, verify the Stop method can be called safely
	al.Stop()

	// Verify running is false (initial state or after Stop)
	if al.running.Load() {
		t.Error("Expected agent to be stopped (or never started)")
	}
}

// Mock implementations for testing

type simpleMockProvider struct {
	response string
}

func (m *simpleMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *simpleMockProvider) GetDefaultModel() string {
	return "mock-model"
}

type reasoningContentProvider struct {
	response         string
	reasoningContent string
}

func (m *reasoningContentProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:          m.response,
		ReasoningContent: m.reasoningContent,
		ToolCalls:        []providers.ToolCall{},
	}, nil
}

func (m *reasoningContentProvider) GetDefaultModel() string {
	return "reasoning-content-model"
}

type countingMockProvider struct {
	response string
	calls    int
}

func (m *countingMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *countingMockProvider) GetDefaultModel() string {
	return "counting-mock-model"
}

type handledMediaProvider struct {
	calls      int
	toolCounts []int
}

func (m *handledMediaProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	m.toolCounts = append(m.toolCounts, len(tools))
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "Taking the screenshot now.",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_handled_media",
				Type:      "function",
				Name:      "handled_media_tool",
				Arguments: map[string]any{},
			}},
		}, nil
	}
	return &providers.LLMResponse{}, nil
}

func (m *handledMediaProvider) GetDefaultModel() string {
	return "handled-media-model"
}

type artifactThenSendProvider struct {
	calls int
}

func (m *artifactThenSendProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "Taking the screenshot now.",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_artifact_media",
				Type:      "function",
				Name:      "media_artifact_tool",
				Arguments: map[string]any{},
			}},
		}, nil
	}

	var artifactPath string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		start := strings.Index(messages[i].Content, "[file:")
		if start < 0 {
			continue
		}
		rest := messages[i].Content[start+len("[file:"):]
		end := strings.Index(rest, "]")
		if end < 0 {
			continue
		}
		artifactPath = rest[:end]
		break
	}
	if artifactPath == "" {
		return nil, fmt.Errorf("provider did not receive artifact path in tool result")
	}

	return &providers.LLMResponse{
		Content: "",
		ToolCalls: []providers.ToolCall{{
			ID:        "call_send_file",
			Type:      "function",
			Name:      "send_file",
			Arguments: map[string]any{"path": artifactPath},
		}},
	}, nil
}

func (m *artifactThenSendProvider) GetDefaultModel() string {
	return "artifact-then-send-model"
}

type toolFeedbackProvider struct {
	filePath string
	calls    int
}

func (m *toolFeedbackProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	if m.calls == 1 {
		return &providers.LLMResponse{
			ToolCalls: []providers.ToolCall{{
				ID:        "call_heartbeat_read_file",
				Type:      "function",
				Name:      "read_file",
				Arguments: map[string]any{"path": m.filePath},
			}},
		}, nil
	}

	return &providers.LLMResponse{
		Content:   "HEARTBEAT_OK",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *toolFeedbackProvider) GetDefaultModel() string {
	return "heartbeat-tool-feedback-model"
}

type picoInterleavedContentProvider struct {
	calls int
}

func (m *picoInterleavedContentProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "intermediate model text",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_tool_limit_test",
				Type:      "function",
				Name:      "tool_limit_test_tool",
				Arguments: map[string]any{"value": "x"},
			}},
		}, nil
	}

	return &providers.LLMResponse{
		Content:   "final model text",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *picoInterleavedContentProvider) GetDefaultModel() string {
	return "pico-interleaved-content-model"
}

type toolLimitOnlyProvider struct{}

func (m *toolLimitOnlyProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		ToolCalls: []providers.ToolCall{{
			ID:        "call_tool_limit_test",
			Type:      "function",
			Name:      "tool_limit_test_tool",
			Arguments: map[string]any{"value": "x"},
		}},
	}, nil
}

func (m *toolLimitOnlyProvider) GetDefaultModel() string {
	return "tool-limit-only-model"
}

// mockCustomTool is a simple mock tool for registration testing
type mockCustomTool struct{}

func (m *mockCustomTool) Name() string {
	return "mock_custom"
}

func (m *mockCustomTool) Description() string {
	return "Mock custom tool for testing"
}

func (m *mockCustomTool) Parameters() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": true,
	}
}

func (m *mockCustomTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.SilentResult("Custom tool executed")
}

type handledMediaTool struct {
	store media.MediaStore
	path  string
}

func (m *handledMediaTool) Name() string { return "handled_media_tool" }
func (m *handledMediaTool) Description() string {
	return "Returns a media attachment and fully handles the user response"
}

func (m *handledMediaTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *handledMediaTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	ref, err := m.store.Store(m.path, media.MediaMeta{
		Filename:    filepath.Base(m.path),
		ContentType: "image/png",
		Source:      "test:handled_media_tool",
	}, "test:handled_media")
	if err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}
	return tools.MediaResult("Attachment delivered by tool.", []string{ref}).WithResponseHandled()
}

type handledMediaWithSteeringProvider struct {
	calls int
}

func (m *handledMediaWithSteeringProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "Taking the screenshot now.",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_handled_media_steering",
				Type:      "function",
				Name:      "handled_media_with_steering_tool",
				Arguments: map[string]any{},
			}},
		}, nil
	}

	for _, msg := range messages {
		if msg.Role == "user" && msg.Content == "what about this instead?" {
			return &providers.LLMResponse{Content: "Handled the queued steering message."}, nil
		}
	}

	return nil, fmt.Errorf("provider did not receive queued steering message")
}

func (m *handledMediaWithSteeringProvider) GetDefaultModel() string {
	return "handled-media-with-steering-model"
}

type handledMediaWithSteeringTool struct {
	store media.MediaStore
	path  string
	loop  *AgentLoop
}

func (m *handledMediaWithSteeringTool) Name() string { return "handled_media_with_steering_tool" }
func (m *handledMediaWithSteeringTool) Description() string {
	return "Returns handled media and enqueues a steering message during execution"
}

func (m *handledMediaWithSteeringTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *handledMediaWithSteeringTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	if err := m.loop.Steer(providers.Message{Role: "user", Content: "what about this instead?"}); err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}

	ref, err := m.store.Store(m.path, media.MediaMeta{
		Filename:    filepath.Base(m.path),
		ContentType: "image/png",
		Source:      "test:handled_media_with_steering_tool",
	}, "test:handled_media_with_steering")
	if err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}
	return tools.MediaResult("Attachment delivered by tool.", []string{ref}).WithResponseHandled()
}

type mediaArtifactTool struct {
	store media.MediaStore
	path  string
}

func (m *mediaArtifactTool) Name() string { return "media_artifact_tool" }
func (m *mediaArtifactTool) Description() string {
	return "Returns a media artifact that the agent can forward or save later"
}

func (m *mediaArtifactTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *mediaArtifactTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	ref, err := m.store.Store(m.path, media.MediaMeta{
		Filename:    filepath.Base(m.path),
		ContentType: "image/png",
		Source:      "test:media_artifact_tool",
	}, "test:media_artifact")
	if err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}
	return tools.MediaResult("Artifact created.", []string{ref})
}

type toolLimitTestTool struct{}

func (m *toolLimitTestTool) Name() string {
	return "tool_limit_test_tool"
}

func (m *toolLimitTestTool) Description() string {
	return "Tool used to exhaust the iteration budget in tests"
}

func (m *toolLimitTestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
	}
}

func (m *toolLimitTestTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.SilentResult("tool limit test result")
}

// testHelper executes a message and returns the response
type testHelper struct {
	al *AgentLoop
}

func newChatCompletionTestServer(
	t *testing.T,
	label string,
	response string,
	calls *int,
	model *string,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("%s server path = %q, want /chat/completions", label, r.URL.Path)
		}
		*calls = *calls + 1
		defer r.Body.Close()

		var req struct {
			Model string `json:"model"`
		}
		decodeErr := json.NewDecoder(r.Body).Decode(&req)
		if decodeErr != nil {
			t.Fatalf("decode %s request: %v", label, decodeErr)
		}
		*model = req.Model

		w.Header().Set("Content-Type", "application/json")
		encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": response},
					"finish_reason": "stop",
				},
			},
		})
		if encodeErr != nil {
			t.Fatalf("encode %s response: %v", label, encodeErr)
		}
	}))
}

func newStrictChatCompletionTestServer(
	t *testing.T,
	label string,
	expectedModel string,
	response string,
	calls *int,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("%s server path = %q, want /chat/completions", label, r.URL.Path)
		}
		*calls = *calls + 1
		defer r.Body.Close()

		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode %s request: %v", label, err)
		}
		if req.Model != expectedModel {
			t.Fatalf("%s server model = %q, want %q", label, req.Model, expectedModel)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": response},
					"finish_reason": "stop",
				},
			},
		}); err != nil {
			t.Fatalf("encode %s response: %v", label, err)
		}
	}))
}

func (h testHelper) executeAndGetResponse(tb testing.TB, ctx context.Context, msg bus.InboundMessage) string {
	// Use a short timeout to avoid hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, responseTimeout)
	defer cancel()

	response, err := h.al.processMessage(timeoutCtx, msg)
	if err != nil {
		tb.Fatalf("processMessage failed: %v", err)
	}
	return response
}

const responseTimeout = 3 * time.Second

func TestProcessMessage_UsesRouteSessionKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &simpleMockProvider{response: "ok"}
	al := NewAgentLoop(cfg, msgBus, provider)

	msg := bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	}

	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel: msg.Channel,
		Peer:    extractPeer(msg),
	})
	sessionKey := route.SessionKey

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}

	helper := testHelper{al: al}
	_ = helper.executeAndGetResponse(t, context.Background(), msg)

	history := defaultAgent.Sessions.GetHistory(sessionKey)
	if len(history) != 2 {
		t.Fatalf("expected session history len=2, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Fatalf("unexpected first message in session: %+v", history[0])
	}
}

func TestProcessMessage_CommandOutcomes(t *testing.T) {
	t.Run("command routing outcomes", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "agent-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Workspace:         tmpDir,
					ModelName:         "test-model",
					MaxTokens:         4096,
					MaxToolIterations: 10,
				},
			},
			Session: config.SessionConfig{
				DMScope: "per-channel-peer",
			},
		}

		msgBus := bus.NewMessageBus()
		provider := &countingMockProvider{response: "LLM reply"}
		al := NewAgentLoop(cfg, msgBus, provider)
		helper := testHelper{al: al}

		baseMsg := bus.InboundMessage{
			Channel:  "telegram",
			SenderID: "user1",
			ChatID:   "chat1",
			Peer: bus.Peer{
				Kind: "direct",
				ID:   "user1",
			},
		}

		showResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
			Channel:  baseMsg.Channel,
			SenderID: baseMsg.SenderID,
			ChatID:   baseMsg.ChatID,
			Content:  "/show channel",
			Peer:     baseMsg.Peer,
		})
		if showResp != "Current Channel: telegram" {
			t.Fatalf("unexpected /show reply: %q", showResp)
		}
		if provider.calls != 0 {
			t.Fatalf("LLM should not be called for handled command, calls=%d", provider.calls)
		}

		fooResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
			Channel:  baseMsg.Channel,
			SenderID: baseMsg.SenderID,
			ChatID:   baseMsg.ChatID,
			Content:  "/foo",
			Peer:     baseMsg.Peer,
		})
		if fooResp != "LLM reply" {
			t.Fatalf("unexpected /foo reply: %q", fooResp)
		}
		if provider.calls != 1 {
			t.Fatalf("LLM should be called exactly once after /foo passthrough, calls=%d", provider.calls)
		}

		newResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
			Channel:  baseMsg.Channel,
			SenderID: baseMsg.SenderID,
			ChatID:   baseMsg.ChatID,
			Content:  "/new",
			Peer:     baseMsg.Peer,
		})
		if newResp != "LLM reply" {
			t.Fatalf("unexpected /new reply: %q", newResp)
		}
		if provider.calls != 2 {
			t.Fatalf("LLM should be called for passthrough /new command, calls=%d", provider.calls)
		}
	})

	t.Run("successful tool execution", func(t *testing.T) {
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
		provider := &multiToolProvider{
			toolCalls: []providers.ToolCall{{
				ID:        "call-1",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			}},
			finalContent: "tool finished",
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "tool finished" {
			t.Fatalf("response = %q, want %q", response, "tool finished")
		}
		if provider.callCount != 2 {
			t.Fatalf("provider call count = %d, want 2", provider.callCount)
		}

		route, _, err := al.resolveMessageRoute(bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("resolveMessageRoute() error = %v", err)
		}
		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(resolveScopeKey(route, ""))
		foundToolMessage := false
		for _, msg := range history {
			if msg.Role == "tool" && strings.Contains(msg.Content, "London") {
				foundToolMessage = true
				break
			}
		}
		if !foundToolMessage {
			t.Fatalf("session history = %#v, want persisted tool result", history)
		}
	})

	t.Run("hook denial", func(t *testing.T) {
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
		provider := &multiToolProvider{
			toolCalls: []providers.ToolCall{{
				ID:        "call-1",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			}},
			finalContent: "after denial",
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		if err := al.MountHook(NamedHook("deny-before", &toolDecisionHook{
			beforeDenied: map[string]string{"lookup_weather": "blocked by before hook"},
		})); err != nil {
			t.Fatalf("MountHook() error = %v", err)
		}

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "after denial" {
			t.Fatalf("response = %q, want %q", response, "after denial")
		}

		want := "Tool execution denied by hook: blocked by before hook"
		route, _, err := al.resolveMessageRoute(bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("resolveMessageRoute() error = %v", err)
		}
		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(resolveScopeKey(route, ""))
		foundDenied := false
		for _, msg := range history {
			if msg.Role == "tool" && msg.Content == want {
				foundDenied = true
				break
			}
		}
		if !foundDenied {
			t.Fatalf("session history = %#v, want denied tool message", history)
		}
	})

	t.Run("approval denial", func(t *testing.T) {
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
		provider := &multiToolProvider{
			toolCalls: []providers.ToolCall{{
				ID:        "call-1",
				Name:      "lookup_weather",
				Arguments: map[string]any{"city": "London"},
			}},
			finalContent: "after approval denial",
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(fakeInteractiveTool{})
		if err := al.MountHook(NamedHook("deny-approval", &toolDecisionHook{
			approvalDenied: map[string]string{"lookup_weather": "approval blocked"},
		})); err != nil {
			t.Fatalf("MountHook() error = %v", err)
		}

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "after approval denial" {
			t.Fatalf("response = %q, want %q", response, "after approval denial")
		}

		want := "Tool execution denied by approval hook: approval blocked"
		route, _, err := al.resolveMessageRoute(bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use the tool",
		})
		if err != nil {
			t.Fatalf("resolveMessageRoute() error = %v", err)
		}
		history := al.GetRegistry().GetDefaultAgent().Sessions.GetHistory(resolveScopeKey(route, ""))
		foundDenied := false
		for _, msg := range history {
			if msg.Role == "tool" && msg.Content == want {
				foundDenied = true
				break
			}
		}
		if !foundDenied {
			t.Fatalf("session history = %#v, want approval denial message", history)
		}
	})

	t.Run("async follow-up publication", func(t *testing.T) {
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
		provider := &multiToolProvider{
			toolCalls: []providers.ToolCall{{
				ID:        "call-async",
				Name:      "async_follow_up",
				Arguments: map[string]any{},
			}},
			finalContent: "done",
		}
		al := NewAgentLoop(cfg, msgBus, provider)
		al.RegisterTool(loopAsyncFollowUpTool{})

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat-1",
			SenderID: "user-1",
			Content:  "use async tool",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "done" {
			t.Fatalf("response = %q, want %q", response, "done")
		}

		select {
		case outbound := <-msgBus.OutboundChan():
			if outbound.Content != "async follow-up ready" {
				t.Fatalf("async outbound content = %q, want %q", outbound.Content, "async follow-up ready")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for async outbound publication")
		}

		select {
		case inbound := <-msgBus.InboundChan():
			if inbound.Channel != "system" {
				t.Fatalf("async inbound channel = %q, want system", inbound.Channel)
			}
			if inbound.SenderID != "async:async_follow_up" {
				t.Fatalf("async inbound sender = %q, want %q", inbound.SenderID, "async:async_follow_up")
			}
			if inbound.Content != "async follow-up ready" {
				t.Fatalf("async inbound content = %q, want %q", inbound.Content, "async follow-up ready")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for async inbound publication")
		}
	})

	t.Run("handled media response", func(t *testing.T) {
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
		provider := &handledMediaProvider{}
		al := NewAgentLoop(cfg, msgBus, provider)

		store := media.NewFileMediaStore()
		al.SetMediaStore(store)
		telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
		al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

		imagePath := filepath.Join(tmpDir, "screen.png")
		if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
			t.Fatalf("WriteFile(imagePath) error = %v", err)
		}

		al.RegisterTool(&handledMediaTool{
			store: store,
			path:  imagePath,
		})

		response, err := al.processMessage(context.Background(), bus.InboundMessage{
			Channel:  "telegram",
			ChatID:   "chat1",
			SenderID: "user1",
			Content:  "take a screenshot of the screen and send it to me",
		})
		if err != nil {
			t.Fatalf("processMessage() error = %v", err)
		}
		if response != "" {
			t.Fatalf("expected no final response when media tool already handled delivery, got %q", response)
		}
		if provider.calls != 1 {
			t.Fatalf("expected exactly 1 LLM call, got %d", provider.calls)
		}
		if len(telegramChannel.sentMedia) != 1 {
			t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
		}

		select {
		case extra := <-msgBus.OutboundMediaChan():
			t.Fatalf("expected handled media to bypass async queue, got %+v", extra)
		default:
		}
	})
}

// TestToolResult_SilentToolDoesNotSendUserMessage verifies silent tools don't trigger outbound
func TestToolResult_SilentToolDoesNotSendUserMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &simpleMockProvider{response: "File operation complete"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ReadFileTool returns SilentResult, which should not send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "read test.txt",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// Silent tool should return the LLM's response directly
	if response != "File operation complete" {
		t.Errorf("Expected 'File operation complete', got: %s", response)
	}
}

// TestToolResult_UserFacingToolDoesSendMessage verifies user-facing tools trigger outbound
func TestToolResult_UserFacingToolDoesSendMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &simpleMockProvider{response: "Command output: hello world"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ExecTool returns UserResult, which should send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "run hello",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// User-facing tool should include the output in final response
	if response != "Command output: hello world" {
		t.Errorf("Expected 'Command output: hello world', got: %s", response)
	}
}

// failFirstMockProvider fails on the first N calls with a specific error
type failFirstMockProvider struct {
	failures    int
	currentCall int
	failError   error
	successResp string
}

func (m *failFirstMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.currentCall++
	if m.currentCall <= m.failures {
		return nil, m.failError
	}
	return &providers.LLMResponse{
		Content:   m.successResp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *failFirstMockProvider) GetDefaultModel() string {
	return "mock-fail-model"
}

// TestAgentLoop_ContextExhaustionRetry verify that the agent retries on context errors
func TestAgentLoop_ContextExhaustionRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	// Create a provider that fails once with a context error
	contextErr := fmt.Errorf("InvalidParameter: Total tokens of image and text exceed max message tokens")
	provider := &failFirstMockProvider{
		failures:    1,
		failError:   contextErr,
		successResp: "Recovered from context error",
	}

	al := NewAgentLoop(cfg, msgBus, provider)

	// Inject some history to simulate a full context.
	// Session history only stores user/assistant/tool messages — the system
	// prompt is built dynamically by BuildMessages and is NOT stored here.
	sessionKey := "test-session-context"
	history := []providers.Message{
		{Role: "user", Content: "Old message 1"},
		{Role: "assistant", Content: "Old response 1"},
		{Role: "user", Content: "Old message 2"},
		{Role: "assistant", Content: "Old response 2"},
		{Role: "user", Content: "Trigger message"},
	}
	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	defaultAgent.Sessions.SetHistory(sessionKey, history)

	// Call ProcessDirectWithChannel
	// Note: ProcessDirectWithChannel calls processMessage which will execute runLLMIteration
	response, err := al.ProcessDirectWithChannel(
		context.Background(),
		"Trigger message",
		sessionKey,
		"test",
		"test-chat",
	)
	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if response != "Recovered from context error" {
		t.Errorf("Expected 'Recovered from context error', got '%s'", response)
	}

	// We expect 2 calls: 1st failed, 2nd succeeded
	if provider.currentCall != 2 {
		t.Errorf("Expected 2 calls (1 fail + 1 success), got %d", provider.currentCall)
	}

	// Check final history length
	finalHistory := defaultAgent.Sessions.GetHistory(sessionKey)
	// We verify that the history has been modified (compressed)
	// Original length: 5
	// Expected behavior: compression drops ~50% of Turns
	// Without compression: 5 + 1 (new user msg) + 1 (assistant msg) = 7
	if len(finalHistory) >= 7 {
		t.Errorf("Expected history to be compressed (len < 7), got %d", len(finalHistory))
	}
}

func TestAgentLoop_EmptyModelResponseUsesAccurateFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 3,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: ""}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.ProcessDirectWithChannel(context.Background(), "hello", "empty-response", "test", "chat1")
	if err != nil {
		t.Fatalf("ProcessDirectWithChannel failed: %v", err)
	}
	if response != defaultResponse {
		t.Fatalf("response = %q, want %q", response, defaultResponse)
	}
}

func TestAgentLoop_ToolLimitUsesDedicatedFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 1,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &toolLimitOnlyProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	al.RegisterTool(&toolLimitTestTool{})

	response, err := al.ProcessDirectWithChannel(context.Background(), "hello", "tool-limit", "test", "chat1")
	if err != nil {
		t.Fatalf("ProcessDirectWithChannel failed: %v", err)
	}
	if response != toolLimitResponse {
		t.Fatalf("response = %q, want %q", response, toolLimitResponse)
	}

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel: "test",
		Peer: &routing.RoutePeer{
			Kind: "direct",
			ID:   "cron",
		},
	})
	history := defaultAgent.Sessions.GetHistory(route.SessionKey)
	if len(history) != 4 {
		t.Fatalf("history len = %d, want 4", len(history))
	}
	assertRoles(t, history, "user", "assistant", "tool", "assistant")
	if history[3].Content != toolLimitResponse {
		t.Fatalf("final assistant content = %q, want %q", history[3].Content, toolLimitResponse)
	}
}

// TestProcessDirectWithChannel_TriggersMCPInitialization verifies that
// ProcessDirectWithChannel triggers MCP initialization when MCP is enabled.
// Note: Manager is only initialized when at least one MCP server is configured
// and successfully connected.
func TestProcessDirectWithChannel_TriggersMCPInitialization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with MCP enabled but no servers - should not initialize manager
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Tools: config.ToolsConfig{
			MCP: config.MCPConfig{
				ToolConfig: config.ToolConfig{
					Enabled: true,
				},
				// No servers configured - manager should not be initialized
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	defer al.Close()

	if al.mcp.hasManager() {
		t.Fatal("expected MCP manager to be nil before first direct processing")
	}

	_, err = al.ProcessDirectWithChannel(
		context.Background(),
		"hello",
		"session-1",
		"cli",
		"direct",
	)
	if err != nil {
		t.Fatalf("ProcessDirectWithChannel failed: %v", err)
	}

	// Manager should not be initialized when no servers are configured
	if al.mcp.hasManager() {
		t.Fatal("expected MCP manager to be nil when no servers are configured")
	}
}

func TestTargetReasoningChannelID_AllChannels(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockProvider{})
	chManager, err := channels.NewManager(&config.Config{}, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}
	for name, id := range map[string]string{
		"unsupported-alpha": "rid-unsupported-alpha",
		"telegram":          "rid-telegram",
		"discord":           "rid-discord",
		"unsupported-beta":  "rid-unsupported-beta",
		"unsupported-gamma": "rid-unsupported-gamma",
		"unsupported-delta": "rid-unsupported-delta",
		"unsupported-eps":   "rid-unsupported-eps",
		"unsupported-zeta":  "rid-unsupported-zeta",
		"unsupported-eta":   "rid-unsupported-eta",
		"unsupported-theta": "rid-unsupported-theta",
		"unsupported-iota":  "rid-unsupported-iota",
	} {
		chManager.RegisterChannel(name, &fakeChannel{id: id})
	}
	al.SetChannelManager(chManager)
	tests := []struct {
		channel string
		wantID  string
	}{
		{channel: "unsupported-alpha", wantID: "rid-unsupported-alpha"},
		{channel: "telegram", wantID: "rid-telegram"},
		{channel: "discord", wantID: "rid-discord"},
		{channel: "unsupported-beta", wantID: "rid-unsupported-beta"},
		{channel: "unsupported-gamma", wantID: "rid-unsupported-gamma"},
		{channel: "unsupported-delta", wantID: "rid-unsupported-delta"},
		{channel: "unsupported-eps", wantID: "rid-unsupported-eps"},
		{channel: "unsupported-zeta", wantID: "rid-unsupported-zeta"},
		{channel: "unsupported-eta", wantID: "rid-unsupported-eta"},
		{channel: "unsupported-theta", wantID: "rid-unsupported-theta"},
		{channel: "unsupported-iota", wantID: "rid-unsupported-iota"},
		{channel: "unknown", wantID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			got := al.targetReasoningChannelID(tt.channel)
			if got != tt.wantID {
				t.Fatalf("targetReasoningChannelID(%q) = %q, want %q", tt.channel, got, tt.wantID)
			}
		})
	}
}

func TestHandleReasoning(t *testing.T) {
	newLoop := func(t *testing.T) (*AgentLoop, *bus.MessageBus) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "agent-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
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
		return NewAgentLoop(cfg, msgBus, &mockProvider{}), msgBus
	}

	t.Run("skips when any required field is empty", func(t *testing.T) {
		al, msgBus := newLoop(t)
		al.handleReasoning(context.Background(), "reasoning", "telegram", "")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for {
			select {
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					t.Fatalf("expected no outbound message, got %+v", msg)
				}
				if msg.Content == "reasoning" {
					t.Fatalf("expected no message for empty chatID, got %+v", msg)
				}
				return
			case <-ctx.Done():
				t.Log("expected an outbound message, got none within timeout")
				return
			default:
				// Continue to check for message
				time.Sleep(5 * time.Millisecond) // Avoid busy loop
			}
		}
	})

	t.Run("publishes one message for non telegram", func(t *testing.T) {
		al, msgBus := newLoop(t)
		al.handleReasoning(context.Background(), "hello reasoning", "discord", "channel-1")

		msg, ok := <-msgBus.OutboundChan()
		if !ok {
			t.Fatal("expected an outbound message")
		}
		if msg.Channel != "discord" || msg.ChatID != "channel-1" || msg.Content != "hello reasoning" {
			t.Fatalf("unexpected outbound message: %+v", msg)
		}
	})

	t.Run("publishes one message for telegram", func(t *testing.T) {
		al, msgBus := newLoop(t)
		reasoning := "hello telegram reasoning"
		al.handleReasoning(context.Background(), reasoning, "telegram", "tg-chat")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				t.Fatal("expected an outbound message, got none within timeout")
				return
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					t.Fatal("expected outbound message")
				}

				if msg.Channel != "telegram" {
					t.Fatalf("expected telegram channel message, got %+v", msg)
				}
				if msg.ChatID != "tg-chat" {
					t.Fatalf("expected chatID tg-chat, got %+v", msg)
				}
				if msg.Content != reasoning {
					t.Fatalf("content mismatch: got %q want %q", msg.Content, reasoning)
				}
				return
			}
		}
	})
	t.Run("expired ctx", func(t *testing.T) {
		al, msgBus := newLoop(t)
		reasoning := "hello telegram reasoning"

		al.handleReasoning(context.Background(), reasoning, "telegram", "tg-chat")

		consumeCtx, consumeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer consumeCancel()

		for {
			select {
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					t.Fatalf("expected no outbound message, but received: %+v", msg)
				}
				t.Logf("Received unexpected outbound message: %+v", msg)
				return
			case <-consumeCtx.Done():
				t.Fatalf("failed: no message received within timeout")
				return
			}
		}
	})

	t.Run("returns promptly when bus is full", func(t *testing.T) {
		al, msgBus := newLoop(t)

		// Fill the outbound bus buffer until a publish would block.
		// Use a short timeout to detect when the buffer is full,
		// rather than hardcoding the buffer size.
		for i := 0; ; i++ {
			fillCtx, fillCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			err := msgBus.PublishOutbound(fillCtx, bus.OutboundMessage{
				Channel: "filler",
				ChatID:  "filler",
				Content: fmt.Sprintf("filler-%d", i),
			})
			fillCancel()
			if err != nil {
				// Buffer is full (timed out trying to send).
				break
			}
		}

		// Use a short-deadline parent context to bound the test.
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		al.handleReasoning(ctx, "should timeout", "discord", "channel-full")
		elapsed := time.Since(start)

		// handleReasoning uses a 5s internal timeout, but the parent ctx
		// expires in 500ms. It should return within ~500ms, not 5s.
		if elapsed > 2*time.Second {
			t.Fatalf("handleReasoning blocked too long (%v); expected prompt return", elapsed)
		}

		// Drain the bus and verify the reasoning message was NOT published
		// (it should have been dropped due to timeout).
		timeer := time.After(1 * time.Second)
		for {
			select {
			case <-timeer:
				t.Logf(
					"no reasoning message received after draining bus for 1s, as expected,length=%d",
					len(msgBus.OutboundChan()),
				)
				return
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					break
				}
				if msg.Content == "should timeout" {
					t.Fatal("expected reasoning message to be dropped when bus is full, but it was published")
				}
			}
		}
	})
}

func TestProcessMessage_PublishesReasoningContentToReasoningChannel(t *testing.T) {
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
	provider := &reasoningContentProvider{
		response:         "final answer",
		reasoningContent: "thinking trace",
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	chManager, err := channels.NewManager(&config.Config{}, msgBus, nil)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}
	chManager.RegisterChannel("telegram", &fakeChannel{id: "reason-chat"})
	al.SetChannelManager(chManager)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "final answer" {
		t.Fatalf("processMessage() response = %q, want %q", response, "final answer")
	}

	select {
	case outbound := <-msgBus.OutboundChan():
		if outbound.Channel != "telegram" {
			t.Fatalf("reasoning channel = %q, want %q", outbound.Channel, "telegram")
		}
		if outbound.ChatID != "reason-chat" {
			t.Fatalf("reasoning chatID = %q, want %q", outbound.ChatID, "reason-chat")
		}
		if outbound.Content != "thinking trace" {
			t.Fatalf("reasoning content = %q, want %q", outbound.Content, "thinking trace")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected reasoning content to be published to reasoning channel")
	}
}

func TestProcessMessage_PicoPublishesReasoningAsThoughtMessage(t *testing.T) {
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
	provider := &reasoningContentProvider{
		response:         "final answer",
		reasoningContent: "thinking trace",
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "pico",
		SenderID: "user1",
		ChatID:   "pico:test-session",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "final answer" {
		t.Fatalf("processMessage() response = %q, want %q", response, "final answer")
	}

	var thoughtMsg *bus.OutboundMessage
	deadline := time.After(3 * time.Second)

	for thoughtMsg == nil {
		select {
		case outbound := <-msgBus.OutboundChan():
			msg := outbound
			if msg.Content == "thinking trace" {
				thoughtMsg = &msg
			}
		case <-deadline:
			t.Fatal("expected thought outbound message for pico")
		}
	}

	if thoughtMsg.Channel != "pico" || thoughtMsg.ChatID != "pico:test-session" {
		t.Fatalf("thought message route = %s/%s, want pico/pico:test-session", thoughtMsg.Channel, thoughtMsg.ChatID)
	}
	if thoughtMsg.Metadata[metadataKeyMessageKind] != messageKindThought {
		t.Fatalf("thought metadata kind = %q, want %q", thoughtMsg.Metadata[metadataKeyMessageKind], messageKindThought)
	}
}

func TestProcessHeartbeat_DoesNotPublishToolFeedback(t *testing.T) {
	tmpDir := t.TempDir()
	heartbeatFile := filepath.Join(tmpDir, "heartbeat-task.txt")
	if err := os.WriteFile(heartbeatFile, []byte("heartbeat task"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
				ToolFeedback: config.ToolFeedbackConfig{
					Enabled:       true,
					MaxArgsLength: 300,
				},
			},
		},
		Tools: config.ToolsConfig{
			ReadFile: config.ReadFileToolConfig{
				Enabled: true,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &toolFeedbackProvider{filePath: heartbeatFile}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.ProcessHeartbeat(context.Background(), "check heartbeat tasks", "telegram", "chat-1")
	if err != nil {
		t.Fatalf("ProcessHeartbeat() error = %v", err)
	}
	if response != "HEARTBEAT_OK" {
		t.Fatalf("ProcessHeartbeat() response = %q, want %q", response, "HEARTBEAT_OK")
	}

	select {
	case outbound := <-msgBus.OutboundChan():
		t.Fatalf("expected no outbound tool feedback during heartbeat, got %+v", outbound)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestProcessMessage_PublishesToolFeedbackWhenEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	heartbeatFile := filepath.Join(tmpDir, "tool-feedback.txt")
	if err := os.WriteFile(heartbeatFile, []byte("tool feedback task"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
				ToolFeedback: config.ToolFeedbackConfig{
					Enabled:       true,
					MaxArgsLength: 300,
				},
			},
		},
		Tools: config.ToolsConfig{
			ReadFile: config.ReadFileToolConfig{
				Enabled: true,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &toolFeedbackProvider{filePath: heartbeatFile}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user-1",
		ChatID:   "chat-1",
		Content:  "check tool feedback",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "HEARTBEAT_OK" {
		t.Fatalf("processMessage() response = %q, want %q", response, "HEARTBEAT_OK")
	}

	select {
	case outbound := <-msgBus.OutboundChan():
		if outbound.Channel != "telegram" {
			t.Fatalf("tool feedback channel = %q, want %q", outbound.Channel, "telegram")
		}
		if outbound.ChatID != "chat-1" {
			t.Fatalf("tool feedback chatID = %q, want %q", outbound.ChatID, "chat-1")
		}
		if !strings.Contains(outbound.Content, "`read_file`") {
			t.Fatalf("tool feedback content = %q, want read_file preview", outbound.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected outbound tool feedback for regular messages")
	}
}

func TestRun_PicoPublishesAssistantContentDuringToolCallsWithoutFinalDuplicate(t *testing.T) {
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
	provider := &picoInterleavedContentProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	agent.Tools.Register(&toolLimitTestTool{})

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- al.Run(runCtx)
	}()

	if err := msgBus.PublishInbound(context.Background(), bus.InboundMessage{
		Channel:  "pico",
		SenderID: "user-1",
		ChatID:   "session-1",
		Content:  "run with tools",
	}); err != nil {
		t.Fatalf("PublishInbound() error = %v", err)
	}

	outputs := make([]string, 0, 2)
	deadline := time.After(2 * time.Second)
	for len(outputs) < 2 {
		select {
		case outbound := <-msgBus.OutboundChan():
			outputs = append(outputs, outbound.Content)
		case <-deadline:
			t.Fatalf("timed out waiting for pico outputs, got %v", outputs)
		}
	}

	if outputs[0] != "intermediate model text" {
		t.Fatalf("first outbound content = %q, want %q", outputs[0], "intermediate model text")
	}
	if outputs[1] != "final model text" {
		t.Fatalf("second outbound content = %q, want %q", outputs[1], "final model text")
	}

	runCancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run() to exit")
	}

	select {
	case outbound := <-msgBus.OutboundChan():
		if outbound.Content == "final model text" {
			t.Fatalf("unexpected duplicate final pico output: %+v", outbound)
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestRunAgentLoop_PicoSkipsInterimPublishWhenNotAllowed(t *testing.T) {
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
	provider := &picoInterleavedContentProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	agent.Tools.Register(&toolLimitTestTool{})

	response, err := al.runAgentLoop(context.Background(), agent, processOptions{
		SessionKey:              "agent:main:pico:session-1",
		Channel:                 "pico",
		ChatID:                  "session-1",
		UserMessage:             "run with tools",
		DefaultResponse:         defaultResponse,
		EnableSummary:           false,
		SendResponse:            false,
		AllowInterimPicoPublish: false,
		SuppressToolFeedback:    true,
	})
	if err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}
	if response != "final model text" {
		t.Fatalf("runAgentLoop() response = %q, want %q", response, "final model text")
	}

	select {
	case outbound := <-msgBus.OutboundChan():
		t.Fatalf("unexpected outbound message when interim publish disabled: %+v", outbound)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestResolveMediaRefs_ResolvesToBase64(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	// Create a minimal valid PNG (8-byte header is enough for filetype detection)
	pngPath := filepath.Join(dir, "test.png")
	// PNG magic: 0x89 P N G \r \n 0x1A \n + minimal IHDR
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, // IHDR length
		0x49, 0x48, 0x44, 0x52, // "IHDR"
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, // 1x1 RGB
		0x00, 0x00, 0x00, // no interlace
		0x90, 0x77, 0x53, 0xDE, // CRC
	}
	if err := os.WriteFile(pngPath, pngHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := store.Store(pngPath, media.MediaMeta{}, "test")
	if err != nil {
		t.Fatal(err)
	}

	messages := []providers.Message{
		{Role: "user", Content: "describe this", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 {
		t.Fatalf("expected 1 resolved media, got %d", len(result[0].Media))
	}
	if !strings.HasPrefix(result[0].Media[0], "data:image/png;base64,") {
		t.Fatalf("expected data:image/png;base64, prefix, got %q", result[0].Media[0][:40])
	}
}

func TestResolveMediaRefs_SkipsOversizedFile(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	bigPath := filepath.Join(dir, "big.png")
	// Write PNG header + padding to exceed limit
	data := make([]byte, 1024+1) // 1KB + 1 byte
	copy(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	if err := os.WriteFile(bigPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ref, _ := store.Store(bigPath, media.MediaMeta{}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	// Use a tiny limit (1KB) so the file is oversized
	result := resolveMediaRefs(messages, store, 1024)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media (oversized), got %d", len(result[0].Media))
	}
}

func TestResolveMediaRefs_UnknownTypeInjectsPath(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	txtPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, _ := store.Store(txtPath, media.MediaMeta{}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media entries, got %d", len(result[0].Media))
	}
	expected := "hi [file:" + txtPath + "]"
	if result[0].Content != expected {
		t.Fatalf("expected content %q, got %q", expected, result[0].Content)
	}
}

func TestResolveMediaRefs_PassesThroughNonMediaRefs(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{"https://example.com/img.png"}},
	}
	result := resolveMediaRefs(messages, nil, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 || result[0].Media[0] != "https://example.com/img.png" {
		t.Fatalf("expected passthrough of non-media:// URL, got %v", result[0].Media)
	}
}

func TestResolveMediaRefs_DoesNotMutateOriginal(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "test.png")
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02,
		0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
	}
	os.WriteFile(pngPath, pngHeader, 0o644)
	ref, _ := store.Store(pngPath, media.MediaMeta{}, "test")

	original := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	originalRef := original[0].Media[0]

	resolveMediaRefs(original, store, config.DefaultMaxMediaSize)

	if original[0].Media[0] != originalRef {
		t.Fatal("resolveMediaRefs mutated original message slice")
	}
}

func TestResolveMediaRefs_UsesMetaContentType(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	// File with JPEG content but stored with explicit content type
	jpegPath := filepath.Join(dir, "photo")
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic bytes
	os.WriteFile(jpegPath, jpegHeader, 0o644)
	ref, _ := store.Store(jpegPath, media.MediaMeta{ContentType: "image/jpeg"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "hi", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(result[0].Media))
	}
	if !strings.HasPrefix(result[0].Media[0], "data:image/jpeg;base64,") {
		t.Fatalf("expected jpeg prefix, got %q", result[0].Media[0][:30])
	}
}

func TestResolveMediaRefs_PDFInjectsFilePath(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	pdfPath := filepath.Join(dir, "report.pdf")
	// PDF magic bytes
	os.WriteFile(pdfPath, []byte("%PDF-1.4 test content"), 0o644)
	ref, _ := store.Store(pdfPath, media.MediaMeta{ContentType: "application/pdf"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "report.pdf [file]", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media (non-image), got %d", len(result[0].Media))
	}
	expected := "report.pdf [file:" + pdfPath + "]"
	if result[0].Content != expected {
		t.Fatalf("expected content %q, got %q", expected, result[0].Content)
	}
}

func TestResolveMediaRefs_AudioInjectsAudioPath(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	oggPath := filepath.Join(dir, "voice.ogg")
	os.WriteFile(oggPath, []byte("fake audio"), 0o644)
	ref, _ := store.Store(oggPath, media.MediaMeta{ContentType: "audio/ogg"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "voice.ogg [audio]", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media, got %d", len(result[0].Media))
	}
	expected := "voice.ogg [audio:" + oggPath + "]"
	if result[0].Content != expected {
		t.Fatalf("expected content %q, got %q", expected, result[0].Content)
	}
}

func TestResolveMediaRefs_VideoInjectsVideoPath(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	mp4Path := filepath.Join(dir, "clip.mp4")
	os.WriteFile(mp4Path, []byte("fake video"), 0o644)
	ref, _ := store.Store(mp4Path, media.MediaMeta{ContentType: "video/mp4"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "clip.mp4 [video]", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 0 {
		t.Fatalf("expected 0 media, got %d", len(result[0].Media))
	}
	expected := "clip.mp4 [video:" + mp4Path + "]"
	if result[0].Content != expected {
		t.Fatalf("expected content %q, got %q", expected, result[0].Content)
	}
}

func TestResolveMediaRefs_NoGenericTagAppendsPath(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	csvPath := filepath.Join(dir, "data.csv")
	os.WriteFile(csvPath, []byte("a,b,c"), 0o644)
	ref, _ := store.Store(csvPath, media.MediaMeta{ContentType: "text/csv"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "here is my data", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	expected := "here is my data [file:" + csvPath + "]"
	if result[0].Content != expected {
		t.Fatalf("expected content %q, got %q", expected, result[0].Content)
	}
}

func TestResolveMediaRefs_EmptyContentGetsPathTag(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	docPath := filepath.Join(dir, "doc.docx")
	os.WriteFile(docPath, []byte("fake docx"), 0o644)
	docxMIME := "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	ref, _ := store.Store(docPath, media.MediaMeta{ContentType: docxMIME}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "", Media: []string{ref}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	expected := "[file:" + docPath + "]"
	if result[0].Content != expected {
		t.Fatalf("expected content %q, got %q", expected, result[0].Content)
	}
}

func TestResolveMediaRefs_MixedImageAndFile(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()

	pngPath := filepath.Join(dir, "photo.png")
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02,
		0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
	}
	os.WriteFile(pngPath, pngHeader, 0o644)
	imgRef, _ := store.Store(pngPath, media.MediaMeta{}, "test")

	pdfPath := filepath.Join(dir, "report.pdf")
	os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0o644)
	fileRef, _ := store.Store(pdfPath, media.MediaMeta{ContentType: "application/pdf"}, "test")

	messages := []providers.Message{
		{Role: "user", Content: "check these [file]", Media: []string{imgRef, fileRef}},
	}
	result := resolveMediaRefs(messages, store, config.DefaultMaxMediaSize)

	if len(result[0].Media) != 1 {
		t.Fatalf("expected 1 media (image only), got %d", len(result[0].Media))
	}
	if !strings.HasPrefix(result[0].Media[0], "data:image/png;base64,") {
		t.Fatal("expected image to be base64 encoded")
	}
	expectedContent := "check these [file:" + pdfPath + "]"
	if result[0].Content != expectedContent {
		t.Fatalf("expected content %q, got %q", expectedContent, result[0].Content)
	}
}

// --- Native search helper tests ---

type nativeSearchProvider struct {
	supported bool
}

func (p *nativeSearchProvider) Chat(
	ctx context.Context, msgs []providers.Message, tools []providers.ToolDefinition,
	model string, opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "ok"}, nil
}

func (p *nativeSearchProvider) GetDefaultModel() string { return "test-model" }

func (p *nativeSearchProvider) SupportsNativeSearch() bool { return p.supported }

type plainProvider struct{}

func (p *plainProvider) Chat(
	ctx context.Context, msgs []providers.Message, tools []providers.ToolDefinition,
	model string, opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "ok"}, nil
}

func (p *plainProvider) GetDefaultModel() string { return "test-model" }

func TestIsNativeSearchProvider_Supported(t *testing.T) {
	if !isNativeSearchProvider(&nativeSearchProvider{supported: true}) {
		t.Fatal("expected true for provider that supports native search")
	}
}

func TestIsNativeSearchProvider_NotSupported(t *testing.T) {
	if isNativeSearchProvider(&nativeSearchProvider{supported: false}) {
		t.Fatal("expected false for provider that does not support native search")
	}
}

func TestIsNativeSearchProvider_NoInterface(t *testing.T) {
	if isNativeSearchProvider(&plainProvider{}) {
		t.Fatal("expected false for provider that does not implement NativeSearchCapable")
	}
}

func TestFilterClientWebSearch_RemovesWebSearch(t *testing.T) {
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "web_search"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "read_file"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "exec"}},
	}
	result := filterClientWebSearch(defs)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	for _, td := range result {
		if td.Function.Name == "web_search" {
			t.Fatal("web_search should be filtered out")
		}
	}
}

func TestFilterClientWebSearch_NoWebSearch(t *testing.T) {
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "read_file"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "exec"}},
	}
	result := filterClientWebSearch(defs)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
}

func TestFilterClientWebSearch_EmptyInput(t *testing.T) {
	result := filterClientWebSearch(nil)
	if len(result) != 0 {
		t.Fatalf("len(result) = %d, want 0", len(result))
	}
}

type overflowProvider struct {
	calls        int
	lastMessages []providers.Message
	chatFunc     func(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]any) (*providers.LLMResponse, error)
}

func (p *overflowProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	p.calls++
	p.lastMessages = append([]providers.Message(nil), messages...)

	if p.chatFunc != nil {
		return p.chatFunc(ctx, messages, tools, model, opts)
	}

	if p.calls == 1 {
		return nil, errors.New("context_window_exceeded")
	}

	return &providers.LLMResponse{
		Content: "Recovered from overflow",
	}, nil
}

func (p *overflowProvider) GetDefaultModel() string {
	return "test-model"
}

func TestProcessMessage_ContextOverflowRecovery(t *testing.T) {
	al, cfg, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()
	_ = cfg

	provider := &overflowProvider{}
	al.registry = NewAgentRegistry(al.cfg, provider)

	sessionKey := "agent:main:test-session"
	agent := al.GetRegistry().GetDefaultAgent()

	for i := 0; i < 5; i++ {
		agent.Sessions.AddFullMessage(sessionKey, providers.Message{Role: "user", Content: "heavy message"})
		agent.Sessions.AddFullMessage(sessionKey, providers.Message{Role: "assistant", Content: "response"})
	}

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:    "test",
		ChatID:     "chat1",
		SenderID:   "user1",
		SessionKey: "test-session",
		Content:    "trigger recovery",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Recovered from overflow" {
		t.Fatalf("response = %q, want %q", response, "Recovered from overflow")
	}

	if provider.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", provider.calls)
	}
}

func TestProcessMessage_ContextOverflow_AnthropicStyle(t *testing.T) {
	al, cfg, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()
	_ = cfg

	provider := &overflowProvider{}
	al.registry = NewAgentRegistry(al.cfg, provider)

	recoveryMsg := "error: status 400: context_window_exceeded"

	provider.chatFunc = func(
		ctx context.Context,
		messages []providers.Message,
		tools []providers.ToolDefinition,
		model string,
		opts map[string]any,
	) (*providers.LLMResponse, error) {
		if provider.calls == 1 {
			return nil, errors.New(recoveryMsg)
		}
		return &providers.LLMResponse{Content: "Anthropic recovery success"}, nil
	}

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "test",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if !strings.Contains(response, "Anthropic recovery success") {
		t.Fatalf("response = %q, want success message", response)
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 calls for retry, got %d", provider.calls)
	}
}
