package agent

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func shouldAutoFallbackToDeepSeek(err error) bool {
	if err == nil {
		return false
	}

	if failErr := providers.ClassifyError(err, "codex", ""); failErr != nil {
		return failErr.Reason == providers.FailoverRateLimit
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "codexruntime: start stdio transport") ||
		strings.Contains(msg, "codexruntime: app-server exited") ||
		strings.Contains(msg, "codexruntime: client not started") ||
		strings.Contains(msg, "codexruntime: initialize") ||
		strings.Contains(msg, "codexruntime: thread/start")
}

func (al *AgentLoop) tryDeepSeekRuntimeFallback(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	toolDefs []providers.ToolDefinition,
	options map[string]any,
	codexErr error,
) (*providers.LLMResponse, bool, error) {
	if agent == nil || agent.DeepSeekFallback == nil || strings.TrimSpace(agent.DeepSeekFallbackModel) == "" {
		return nil, false, nil
	}
	if !shouldAutoFallbackToDeepSeek(codexErr) {
		return nil, false, nil
	}

	resp, err := agent.DeepSeekFallback.Chat(ctx, messages, toolDefs, agent.DeepSeekFallbackModel, options)
	if err != nil {
		return nil, true, err
	}
	return resp, true, nil
}
