package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/thomas-quant/codex-claw/pkg/providers/protocoltypes"
)

type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	LLMResponse            = protocoltypes.LLMResponse
	UsageInfo              = protocoltypes.UsageInfo
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	ExtraContent           = protocoltypes.ExtraContent
	GoogleExtra            = protocoltypes.GoogleExtra
	ContentBlock           = protocoltypes.ContentBlock
	CacheControl           = protocoltypes.CacheControl
)

type LLMProvider interface {
	Chat(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
	) (*LLMResponse, error)
	GetDefaultModel() string
}

type InteractiveContentItem struct {
	Type     string
	Text     string
	ImageURL string
}

type InteractiveInputItem struct {
	Type string
	Text string
	URL  string
	Path string
}

type InteractiveSandboxPolicy struct {
	Mode          string
	WritableRoots []string
	NetworkAccess bool
}

type InteractiveToolCall struct {
	CallID    string
	Name      string
	Arguments map[string]any
}

type InteractiveToolResult struct {
	ContentItems []InteractiveContentItem
	Success      bool
}

type InteractiveToolExecutor func(context.Context, InteractiveToolCall) (InteractiveToolResult, error)

type InteractiveRecoveryRequest struct {
	AllowServerRestart bool
	AllowResume        bool
}

type InteractiveControlRequest struct {
	ThinkingMode      string
	FastEnabled       bool
	FastEnabledSet    bool
	LastUserMessageAt string
	ForceFreshThread  bool
}

type InteractiveThreadControlRequest struct {
	SessionKey string
	AgentID    string
	Channel    string
	ChatID     string
}

type InteractiveModelInfo struct {
	ID            string
	Label         string
	Provider      string
	ThinkingModes []string
	SpeedTier     string
}

type InteractiveThreadStatus struct {
	ThreadID             string
	Model                string
	Provider             string
	ThinkingMode         string
	FastEnabled          bool
	RecoveryState        string
	LastUserMessageAt    time.Time
	LastCompactionAt     time.Time
	ForceFreshThread     bool
	ActiveAccountAlias   string
	AccountHealth        string
	TelemetryFresh       bool
	FiveHourRemainingPct int
	WeeklyRemainingPct   int
	SwitchTrigger        string
}

// InteractiveTurnRequest carries the provider-layer state needed to execute
// a stateful interactive turn without changing the base LLMProvider interface.
type InteractiveTurnRequest struct {
	SessionKey             string
	AgentID                string
	Channel                string
	ChatID                 string
	Model                  string
	Messages               []Message
	Tools                  []ToolDefinition
	Options                map[string]any
	BootstrapInput         string
	RecoveryBootstrapInput string
	InputItems             []InteractiveInputItem
	SandboxPolicy          *InteractiveSandboxPolicy
	Recovery               InteractiveRecoveryRequest
	Control                InteractiveControlRequest
	OnChunk                func(string)
	ExecuteTool            InteractiveToolExecutor
}

// InteractiveProvider is an optional capability for providers that manage a
// stateful turn loop outside the stateless Chat interface.
type InteractiveProvider interface {
	RunInteractiveTurn(context.Context, InteractiveTurnRequest) (*LLMResponse, error)
}

type InteractiveTurnSteerer interface {
	SteerInteractiveTurn(context.Context, InteractiveThreadControlRequest, []InteractiveInputItem) error
}

type InteractiveThreadController interface {
	CompactThread(context.Context, InteractiveThreadControlRequest) error
}

type InteractiveRuntimeController interface {
	InteractiveThreadController
	ListModels(context.Context) ([]InteractiveModelInfo, error)
	ReadThreadStatus(context.Context, InteractiveThreadControlRequest) (InteractiveThreadStatus, error)
	SetThreadModel(context.Context, InteractiveThreadControlRequest, string) (string, error)
	SetThreadThinking(context.Context, InteractiveThreadControlRequest, string) (string, error)
	ToggleThreadFast(context.Context, InteractiveThreadControlRequest) (bool, error)
	ResetThread(context.Context, InteractiveThreadControlRequest) error
}

type StatefulProvider interface {
	LLMProvider
	Close()
}

// StreamingProvider is an optional interface for providers that support token streaming.
// onChunk receives the accumulated text so far (not individual deltas).
// The returned LLMResponse is the same complete response for compatibility with tool-call handling.
type StreamingProvider interface {
	ChatStream(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
		onChunk func(accumulated string),
	) (*LLMResponse, error)
}

// ThinkingCapable is an optional interface for providers that support
// extended thinking (e.g. Anthropic). Used by the agent loop to warn
// when thinking_level is configured but the active provider cannot use it.
type ThinkingCapable interface {
	SupportsThinking() bool
}

// NativeSearchCapable is an optional interface for providers that support
// built-in web search during LLM inference (e.g. OpenAI web_search_preview,
// xAI Grok search). When the active provider implements this interface and
// returns true, the agent loop can hide the client-side web_search tool to
// avoid duplicate search surfaces and use the provider's native search instead.
type NativeSearchCapable interface {
	SupportsNativeSearch() bool
}

// FailoverReason classifies why an LLM request failed for fallback decisions.
type FailoverReason string

const (
	FailoverAuth            FailoverReason = "auth"
	FailoverRateLimit       FailoverReason = "rate_limit"
	FailoverBilling         FailoverReason = "billing"
	FailoverTimeout         FailoverReason = "timeout"
	FailoverFormat          FailoverReason = "format"
	FailoverContextOverflow FailoverReason = "context_overflow"
	FailoverOverloaded      FailoverReason = "overloaded"
	FailoverUnknown         FailoverReason = "unknown"
)

// FailoverError wraps an LLM provider error with classification metadata.
type FailoverError struct {
	Reason   FailoverReason
	Provider string
	Model    string
	Status   int
	Wrapped  error
}

func (e *FailoverError) Error() string {
	return fmt.Sprintf("failover(%s): provider=%s model=%s status=%d: %v",
		e.Reason, e.Provider, e.Model, e.Status, e.Wrapped)
}

func (e *FailoverError) Unwrap() error {
	return e.Wrapped
}

// IsRetriable returns true if this error should trigger fallback to next candidate.
// Non-retriable: Format errors (bad request structure, image dimension/size).
func (e *FailoverError) IsRetriable() bool {
	return e.Reason != FailoverFormat && e.Reason != FailoverContextOverflow
}

// ModelConfig holds primary model and fallback list.
type ModelConfig struct {
	Primary   string
	Fallbacks []string
}
