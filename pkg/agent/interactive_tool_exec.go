package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

func (al *AgentLoop) executeInteractiveToolCall(
	ctx context.Context,
	ts *turnState,
	call providers.InteractiveToolCall,
) (providers.InteractiveToolResult, error) {
	if ts == nil || ts.agent == nil {
		return providers.InteractiveToolResult{}, fmt.Errorf("interactive tool execution missing turn state")
	}

	iteration := ts.currentIteration()
	toolName := call.Name
	toolArgs := cloneStringAnyMap(call.Arguments)

	al.persistInteractiveAssistantToolCall(ctx, ts, call)

	if al.hooks != nil {
		toolReq, decision := al.hooks.BeforeTool(ctx, &ToolCallHookRequest{
			Meta:      ts.eventMeta("runInteractiveTool", "turn.tool.before"),
			Tool:      toolName,
			Arguments: toolArgs,
			Channel:   ts.channel,
			ChatID:    ts.chatID,
		})
		switch decision.normalizedAction() {
		case HookActionContinue, HookActionModify:
			if toolReq != nil {
				toolName = toolReq.Tool
				toolArgs = toolReq.Arguments
			}
		case HookActionRespond:
			if toolReq != nil && toolReq.HookResult != nil {
				return al.finishInteractiveToolCall(ctx, ts, call.CallID, toolName, toolReq.HookResult, 0)
			}
		case HookActionDenyTool:
			denyContent := hookDeniedToolContent("Tool execution denied by hook", decision.Reason)
			return al.persistInteractiveToolFailure(ctx, ts, call.CallID, toolName, denyContent), nil
		case HookActionAbortTurn:
			return providers.InteractiveToolResult{}, al.hookAbortError(ts, "before_tool", decision)
		case HookActionHardAbort:
			_ = ts.requestHardAbort()
			return providers.InteractiveToolResult{}, context.Canceled
		}
	}

	if al.hooks != nil {
		approval := al.hooks.ApproveTool(ctx, &ToolApprovalRequest{
			Meta:      ts.eventMeta("runInteractiveTool", "turn.tool.approve"),
			Tool:      toolName,
			Arguments: toolArgs,
			Channel:   ts.channel,
			ChatID:    ts.chatID,
		})
		if !approval.Approved {
			denyContent := hookDeniedToolContent("Tool execution denied by approval hook", approval.Reason)
			return al.persistInteractiveToolFailure(ctx, ts, call.CallID, toolName, denyContent), nil
		}
	}

	argsJSON, _ := json.Marshal(toolArgs)
	argsPreview := utils.Truncate(string(argsJSON), 200)
	logger.InfoCF("agent", fmt.Sprintf("Interactive tool call: %s(%s)", toolName, argsPreview),
		map[string]any{
			"agent_id":  ts.agent.ID,
			"tool":      toolName,
			"iteration": iteration,
		})
	al.emitEvent(
		EventKindToolExecStart,
		ts.eventMeta("runInteractiveTool", "turn.tool.start"),
		ToolExecStartPayload{
			Tool:      toolName,
			Arguments: cloneEventArguments(toolArgs),
		},
	)

	if al.cfg.Agents.Defaults.IsToolFeedbackEnabled() &&
		ts.channel != "" &&
		!ts.opts.SuppressToolFeedback {
		feedbackPreview := utils.Truncate(
			string(argsJSON),
			al.cfg.Agents.Defaults.GetToolFeedbackMaxArgsLength(),
		)
		feedbackMsg := utils.FormatToolFeedbackMessage(toolName, feedbackPreview)
		fbCtx, fbCancel := context.WithTimeout(ctx, 3*time.Second)
		_ = al.bus.PublishOutbound(fbCtx, bus.OutboundMessage{
			Channel: ts.channel,
			ChatID:  ts.chatID,
			Content: feedbackMsg,
		})
		fbCancel()
	}

	toolIteration := iteration
	asyncToolName := toolName
	asyncCallback := func(_ context.Context, result *tools.ToolResult) {
		if !result.Silent && result.ForUser != "" {
			outCtx, outCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer outCancel()
			_ = al.bus.PublishOutbound(outCtx, bus.OutboundMessage{
				Channel: ts.channel,
				ChatID:  ts.chatID,
				Content: result.ForUser,
			})
		}

		content := result.ContentForLLM()
		if content == "" {
			return
		}

		content = al.cfg.FilterSensitiveData(content)
		al.emitEvent(
			EventKindFollowUpQueued,
			ts.scope.meta(toolIteration, "runInteractiveTool", "turn.follow_up.queued"),
			FollowUpQueuedPayload{
				SourceTool: asyncToolName,
				Channel:    ts.channel,
				ChatID:     ts.chatID,
				ContentLen: len(content),
			},
		)

		pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pubCancel()
		_ = al.bus.PublishInbound(pubCtx, bus.InboundMessage{
			Channel:  "system",
			SenderID: fmt.Sprintf("async:%s", asyncToolName),
			ChatID:   fmt.Sprintf("%s:%s", ts.channel, ts.chatID),
			Content:  content,
		})
	}

	execCtx := tools.WithToolInboundContext(
		ctx,
		ts.channel,
		ts.chatID,
		ts.opts.MessageID,
		ts.opts.ReplyToMessageID,
	)
	toolStart := time.Now()
	toolResult := ts.agent.Tools.ExecuteWithContext(
		execCtx,
		toolName,
		toolArgs,
		ts.channel,
		ts.chatID,
		asyncCallback,
	)
	toolDuration := time.Since(toolStart)

	if al.hooks != nil {
		toolResp, decision := al.hooks.AfterTool(ctx, &ToolResultHookResponse{
			Meta:      ts.eventMeta("runInteractiveTool", "turn.tool.after"),
			Tool:      toolName,
			Arguments: toolArgs,
			Result:    toolResult,
			Duration:  toolDuration,
			Channel:   ts.channel,
			ChatID:    ts.chatID,
		})
		switch decision.normalizedAction() {
		case HookActionContinue, HookActionModify:
			if toolResp != nil {
				if toolResp.Tool != "" {
					toolName = toolResp.Tool
				}
				if toolResp.Result != nil {
					toolResult = toolResp.Result
				}
			}
		case HookActionAbortTurn:
			return providers.InteractiveToolResult{}, al.hookAbortError(ts, "after_tool", decision)
		case HookActionHardAbort:
			_ = ts.requestHardAbort()
			return providers.InteractiveToolResult{}, context.Canceled
		}
	}

	if toolResult == nil {
		toolResult = tools.ErrorResult("hook returned nil tool result")
	}

	return al.finishInteractiveToolCall(ctx, ts, call.CallID, toolName, toolResult, toolDuration)
}

func (al *AgentLoop) persistInteractiveAssistantToolCall(ctx context.Context, ts *turnState, call providers.InteractiveToolCall) {
	argumentsJSON, _ := json.Marshal(call.Arguments)
	assistantMsg := providers.Message{
		Role: "assistant",
		ToolCalls: []providers.ToolCall{{
			ID:   call.CallID,
			Type: "function",
			Name: call.Name,
			Function: &providers.FunctionCall{
				Name:      call.Name,
				Arguments: string(argumentsJSON),
			},
		}},
	}
	if !ts.opts.NoHistory {
		ts.agent.Sessions.AddFullMessage(ts.sessionKey, assistantMsg)
		ts.recordPersistedMessage(assistantMsg)
		ts.ingestMessage(ctx, al, assistantMsg)
	}
}

func (al *AgentLoop) persistInteractiveToolFailure(
	ctx context.Context,
	ts *turnState,
	toolCallID string,
	toolName string,
	content string,
) providers.InteractiveToolResult {
	al.emitEvent(
		EventKindToolExecSkipped,
		ts.eventMeta("runInteractiveTool", "turn.tool.skipped"),
		ToolExecSkippedPayload{
			Tool:   toolName,
			Reason: content,
		},
	)
	deniedMsg := providers.Message{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}
	if !ts.opts.NoHistory {
		ts.agent.Sessions.AddFullMessage(ts.sessionKey, deniedMsg)
		ts.recordPersistedMessage(deniedMsg)
		ts.ingestMessage(ctx, al, deniedMsg)
	}
	return providers.InteractiveToolResult{
		Success: false,
		ContentItems: []providers.InteractiveContentItem{
			{Type: "text", Text: content},
		},
	}
}

func (al *AgentLoop) finishInteractiveToolCall(
	ctx context.Context,
	ts *turnState,
	toolCallID string,
	toolName string,
	toolResult *tools.ToolResult,
	toolDuration time.Duration,
) (providers.InteractiveToolResult, error) {
	if toolResult == nil {
		toolResult = tools.ErrorResult("tool returned nil result unexpectedly")
	}

	shouldSendForUser := !toolResult.Silent && toolResult.ForUser != "" &&
		(ts.opts.SendResponse || toolResult.ResponseHandled)
	if shouldSendForUser {
		al.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: ts.channel,
			ChatID:  ts.chatID,
			Content: toolResult.ForUser,
			Metadata: map[string]string{
				"is_tool_call": "true",
			},
		})
	}

	if len(toolResult.Media) > 0 && toolResult.ResponseHandled {
		parts := make([]bus.MediaPart, 0, len(toolResult.Media))
		for _, ref := range toolResult.Media {
			part := bus.MediaPart{Ref: ref}
			if al.mediaStore != nil {
				if _, meta, err := al.mediaStore.ResolveWithMeta(ref); err == nil {
					part.Filename = meta.Filename
					part.ContentType = meta.ContentType
					part.Type = inferMediaType(meta.Filename, meta.ContentType)
				}
			}
			parts = append(parts, part)
		}
		outboundMedia := bus.OutboundMediaMessage{
			Channel: ts.channel,
			ChatID:  ts.chatID,
			Parts:   parts,
		}
		if al.channelManager != nil && ts.channel != "" && !constants.IsInternalChannel(ts.channel) {
			if err := al.channelManager.SendMedia(ctx, outboundMedia); err != nil {
				logger.WarnCF("agent", "Failed to deliver handled tool media",
					map[string]any{
						"agent_id": ts.agent.ID,
						"tool":     toolName,
						"channel":  ts.channel,
						"chat_id":  ts.chatID,
						"error":    err.Error(),
					})
				toolResult = tools.ErrorResult(fmt.Sprintf("failed to deliver attachment: %v", err)).WithError(err)
			}
		} else if al.bus != nil {
			al.bus.PublishOutboundMedia(ctx, outboundMedia)
			toolResult.ResponseHandled = false
		}
	}

	if len(toolResult.Media) > 0 && !toolResult.ResponseHandled {
		toolResult.ArtifactTags = buildArtifactTags(al.mediaStore, toolResult.Media)
	}

	contentForLLM := toolResult.ContentForLLM()
	if al.cfg.Tools.IsFilterSensitiveDataEnabled() {
		contentForLLM = al.cfg.FilterSensitiveData(contentForLLM)
	}

	toolResultMsg := providers.Message{
		Role:       "tool",
		Content:    contentForLLM,
		ToolCallID: toolCallID,
	}
	if len(toolResult.Media) > 0 && !toolResult.ResponseHandled {
		toolResultMsg.Media = append(toolResultMsg.Media, toolResult.Media...)
	}

	al.emitEvent(
		EventKindToolExecEnd,
		ts.eventMeta("runInteractiveTool", "turn.tool.end"),
		ToolExecEndPayload{
			Tool:       toolName,
			Duration:   toolDuration,
			ForLLMLen:  len(contentForLLM),
			ForUserLen: len(toolResult.ForUser),
			IsError:    toolResult.IsError,
			Async:      toolResult.Async,
		},
	)

	if !ts.opts.NoHistory {
		ts.agent.Sessions.AddFullMessage(ts.sessionKey, toolResultMsg)
		ts.recordPersistedMessage(toolResultMsg)
		ts.ingestMessage(ctx, al, toolResultMsg)
	}

	result := providers.InteractiveToolResult{
		Success: !toolResult.IsError,
	}
	if contentForLLM != "" {
		result.ContentItems = append(result.ContentItems, providers.InteractiveContentItem{
			Type: "text",
			Text: contentForLLM,
		})
	}

	return result, nil
}
