package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/codexruntime"
)

const codexAppServerDefaultModel = "codex-app-server"

type codexAppServerRunner interface {
	RunTextTurn(context.Context, codexruntime.RunRequest) (codexruntime.RunResult, error)
	CompactThread(context.Context, string) error
	ListModels(context.Context) ([]codexruntime.ModelCatalogEntry, error)
	ReadStatus(context.Context, string) (codexruntime.RuntimeStatusSnapshot, error)
	SetModel(context.Context, string, string) (string, error)
	SetThinkingMode(context.Context, string, string) (string, error)
	ToggleFast(context.Context, string) (bool, error)
	ResetThread(context.Context, string) error
}

// CodexAppServerProvider adapts the codex runtime runner to the provider layer.
type CodexAppServerProvider struct {
	runner codexAppServerRunner
}

func NewCodexAppServerProvider(runner codexAppServerRunner) *CodexAppServerProvider {
	return &CodexAppServerProvider{runner: runner}
}

func (p *CodexAppServerProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	return p.RunInteractiveTurn(ctx, InteractiveTurnRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Options:  options,
	})
}

func (p *CodexAppServerProvider) GetDefaultModel() string {
	return codexAppServerDefaultModel
}

func (p *CodexAppServerProvider) RunInteractiveTurn(
	ctx context.Context,
	req InteractiveTurnRequest,
) (*LLMResponse, error) {
	if p.runner == nil {
		return nil, fmt.Errorf("codex app server runner not configured")
	}

	inputText := req.BootstrapInput
	if inputText == "" {
		inputText = lastUserMessageContent(req.Messages)
	}
	if inputText == "" {
		return nil, fmt.Errorf("interactive turn requires bootstrap input or a user message")
	}

	control := codexruntime.ControlRequest{
		ThinkingMode:      req.Control.ThinkingMode,
		FastEnabled:       req.Control.FastEnabled,
		FastEnabledSet:    req.Control.FastEnabledSet,
		LastUserMessageAt: req.Control.LastUserMessageAt,
		ForceFreshThread:  req.Control.ForceFreshThread,
	}

	result, err := p.runner.RunTextTurn(ctx, codexruntime.RunRequest{
		BindingKey: interactiveBindingKey(req),
		Model:      req.Model,
		InputText:  inputText,
		Recovery: codexruntime.RecoveryRequest{
			AllowServerRestart: req.Recovery.AllowServerRestart,
			AllowResume:        req.Recovery.AllowResume,
		},
		Control:        control,
		DynamicTools:   mapDynamicTools(req.Tools),
		HandleToolCall: mapInteractiveToolExecutor(req.ExecuteTool),
		OnChunk:        req.OnChunk,
	})
	if err != nil {
		return nil, err
	}

	return &LLMResponse{
		Content:      result.Content,
		FinishReason: "stop",
	}, nil
}

func (p *CodexAppServerProvider) CompactThread(ctx context.Context, req InteractiveThreadControlRequest) error {
	if p.runner == nil {
		return fmt.Errorf("codex app server runner not configured")
	}

	return p.runner.CompactThread(ctx, interactiveBindingKey(InteractiveTurnRequest{
		SessionKey: req.SessionKey,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
	}))
}

func (p *CodexAppServerProvider) ListModels(ctx context.Context) ([]InteractiveModelInfo, error) {
	if p.runner == nil {
		return nil, fmt.Errorf("codex app server runner not configured")
	}

	models, err := p.runner.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]InteractiveModelInfo, 0, len(models))
	for _, model := range models {
		out = append(out, InteractiveModelInfo{
			ID:            model.ID,
			Label:         model.Label,
			Provider:      "codex",
			ThinkingModes: append([]string(nil), model.ReasoningEffortOptions...),
			SpeedTier:     model.SpeedTier,
		})
	}
	return out, nil
}

func (p *CodexAppServerProvider) ReadThreadStatus(ctx context.Context, req InteractiveThreadControlRequest) (InteractiveThreadStatus, error) {
	if p.runner == nil {
		return InteractiveThreadStatus{}, fmt.Errorf("codex app server runner not configured")
	}

	status, err := p.runner.ReadStatus(ctx, interactiveBindingKey(InteractiveTurnRequest{
		SessionKey: req.SessionKey,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
	}))
	if err != nil {
		return InteractiveThreadStatus{}, err
	}

	return InteractiveThreadStatus{
		ThreadID:          status.ThreadID,
		Model:             status.Model,
		Provider:          "codex",
		ThinkingMode:      status.ThinkingMode,
		FastEnabled:       status.FastEnabled,
		RecoveryState:     status.Recovery.Mode,
		LastUserMessageAt: status.LastUserMessageAt,
		LastCompactionAt:  status.LastCompactionAt,
		ForceFreshThread:  status.ForceFreshThread,
	}, nil
}

func (p *CodexAppServerProvider) SetThreadModel(ctx context.Context, req InteractiveThreadControlRequest, model string) (string, error) {
	if p.runner == nil {
		return "", fmt.Errorf("codex app server runner not configured")
	}
	return p.runner.SetModel(ctx, interactiveBindingKey(InteractiveTurnRequest{
		SessionKey: req.SessionKey,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
	}), model)
}

func (p *CodexAppServerProvider) SetThreadThinking(ctx context.Context, req InteractiveThreadControlRequest, thinking string) (string, error) {
	if p.runner == nil {
		return "", fmt.Errorf("codex app server runner not configured")
	}
	return p.runner.SetThinkingMode(ctx, interactiveBindingKey(InteractiveTurnRequest{
		SessionKey: req.SessionKey,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
	}), thinking)
}

func (p *CodexAppServerProvider) ToggleThreadFast(ctx context.Context, req InteractiveThreadControlRequest) (bool, error) {
	if p.runner == nil {
		return false, fmt.Errorf("codex app server runner not configured")
	}
	return p.runner.ToggleFast(ctx, interactiveBindingKey(InteractiveTurnRequest{
		SessionKey: req.SessionKey,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
	}))
}

func (p *CodexAppServerProvider) ResetThread(ctx context.Context, req InteractiveThreadControlRequest) error {
	if p.runner == nil {
		return fmt.Errorf("codex app server runner not configured")
	}
	return p.runner.ResetThread(ctx, interactiveBindingKey(InteractiveTurnRequest{
		SessionKey: req.SessionKey,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
	}))
}

func lastUserMessageContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}

	return ""
}

func mapDynamicTools(tools []ToolDefinition) []codexruntime.DynamicToolDefinition {
	if len(tools) == 0 {
		return nil
	}

	mapped := make([]codexruntime.DynamicToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if tool.Function.Name == "" {
			continue
		}
		mapped = append(mapped, codexruntime.DynamicToolDefinition{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}

	return mapped
}

func mapInteractiveToolExecutor(executor InteractiveToolExecutor) codexruntime.ToolCallHandler {
	if executor == nil {
		return nil
	}

	return func(ctx context.Context, call codexruntime.ToolCallRequest) (codexruntime.ToolCallResult, error) {
		result, err := executor(ctx, InteractiveToolCall{
			CallID:    call.CallID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
		if err != nil {
			return codexruntime.ToolCallResult{}, err
		}

		content := make([]codexruntime.ToolResultContentItem, 0, len(result.ContentItems))
		for _, item := range result.ContentItems {
			content = append(content, codexruntime.ToolResultContentItem{
				Type:     item.Type,
				Text:     item.Text,
				ImageURL: item.ImageURL,
			})
		}

		return codexruntime.ToolCallResult{
			Content: content,
			Success: result.Success,
		}, nil
	}
}

func interactiveBindingKey(req InteractiveTurnRequest) string {
	if req.SessionKey != "" {
		if req.AgentID != "" {
			return req.SessionKey + ":" + req.AgentID
		}
		return req.SessionKey
	}

	if req.Channel == "" && req.ChatID == "" && req.AgentID == "" {
		return ""
	}

	return strings.Join([]string{req.Channel, req.ChatID, req.AgentID}, ":")
}
