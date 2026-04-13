package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/codex-claw/pkg/bus"
	"github.com/sipeed/codex-claw/pkg/constants"
	"github.com/sipeed/codex-claw/pkg/logger"
	"github.com/sipeed/codex-claw/pkg/providers"
	"github.com/sipeed/codex-claw/pkg/tools"
	"github.com/sipeed/codex-claw/pkg/utils"
)

type beforeToolHookOutcome struct {
	toolName    string
	toolArgs    map[string]any
	hookResult  *tools.ToolResult
	denyContent string
	hardAbort   bool
}

type afterToolHookOutcome struct {
	toolName   string
	toolResult *tools.ToolResult
	hardAbort  bool
}

type finalizedToolExecution struct {
	toolResult    *tools.ToolResult
	toolMessage   providers.Message
	contentForLLM string
}

func (f finalizedToolExecution) interactiveResult() providers.InteractiveToolResult {
	result := providers.InteractiveToolResult{
		Success: f.toolResult != nil && !f.toolResult.IsError,
	}
	if f.contentForLLM != "" {
		result.ContentItems = append(result.ContentItems, providers.InteractiveContentItem{
			Type: "text",
			Text: f.contentForLLM,
		})
	}
	return result
}

func (al *AgentLoop) applyBeforeToolHooks(
	ctx context.Context,
	ts *turnState,
	source string,
	toolName string,
	toolArgs map[string]any,
	warnOnMissingRespond bool,
) (beforeToolHookOutcome, error) {
	outcome := beforeToolHookOutcome{
		toolName: toolName,
		toolArgs: toolArgs,
	}
	if al.hooks == nil {
		return outcome, nil
	}

	toolReq, decision := al.hooks.BeforeTool(ctx, &ToolCallHookRequest{
		Meta:      ts.eventMeta(source, "turn.tool.before"),
		Tool:      toolName,
		Arguments: toolArgs,
		Channel:   ts.channel,
		ChatID:    ts.chatID,
	})
	switch decision.normalizedAction() {
	case HookActionContinue, HookActionModify:
		if toolReq != nil {
			outcome.toolName = toolReq.Tool
			outcome.toolArgs = toolReq.Arguments
		}
	case HookActionRespond:
		if toolReq != nil && toolReq.HookResult != nil {
			outcome.hookResult = toolReq.HookResult
			return outcome, nil
		}
		if warnOnMissingRespond {
			logger.WarnCF("agent", "Hook returned respond action but no HookResult provided",
				map[string]any{
					"agent_id": ts.agent.ID,
					"tool":     toolName,
					"action":   "respond",
				})
		}
	case HookActionDenyTool:
		outcome.denyContent = hookDeniedToolContent("Tool execution denied by hook", decision.Reason)
	case HookActionAbortTurn:
		return outcome, al.hookAbortError(ts, "before_tool", decision)
	case HookActionHardAbort:
		_ = ts.requestHardAbort()
		outcome.hardAbort = true
	}

	return outcome, nil
}

func (al *AgentLoop) approveToolExecution(
	ctx context.Context,
	ts *turnState,
	source string,
	toolName string,
	toolArgs map[string]any,
) string {
	if al.hooks == nil {
		return ""
	}

	approval := al.hooks.ApproveTool(ctx, &ToolApprovalRequest{
		Meta:      ts.eventMeta(source, "turn.tool.approve"),
		Tool:      toolName,
		Arguments: toolArgs,
		Channel:   ts.channel,
		ChatID:    ts.chatID,
	})
	if approval.Approved {
		return ""
	}
	return hookDeniedToolContent("Tool execution denied by approval hook", approval.Reason)
}

func (al *AgentLoop) emitToolExecutionStart(
	ctx context.Context,
	ts *turnState,
	source string,
	logLabel string,
	toolName string,
	feedbackToolName string,
	toolArgs map[string]any,
) {
	argsJSON, _ := json.Marshal(toolArgs)
	argsPreview := utils.Truncate(string(argsJSON), 200)
	logger.InfoCF("agent", fmt.Sprintf("%s: %s(%s)", logLabel, toolName, argsPreview),
		map[string]any{
			"agent_id":  ts.agent.ID,
			"tool":      toolName,
			"iteration": ts.currentIteration(),
		})
	al.emitEvent(
		EventKindToolExecStart,
		ts.eventMeta(source, "turn.tool.start"),
		ToolExecStartPayload{
			Tool:      toolName,
			Arguments: cloneEventArguments(toolArgs),
		},
	)

	if al.cfg.Agents.Defaults.IsToolFeedbackEnabled() &&
		ts.channel != "" &&
		!ts.opts.SuppressToolFeedback {
		if feedbackToolName == "" {
			feedbackToolName = toolName
		}
		feedbackPreview := utils.Truncate(
			string(argsJSON),
			al.cfg.Agents.Defaults.GetToolFeedbackMaxArgsLength(),
		)
		feedbackMsg := utils.FormatToolFeedbackMessage(feedbackToolName, feedbackPreview)
		fbCtx, fbCancel := context.WithTimeout(ctx, 3*time.Second)
		_ = al.bus.PublishOutbound(fbCtx, bus.OutboundMessage{
			Channel: ts.channel,
			ChatID:  ts.chatID,
			Content: feedbackMsg,
		})
		fbCancel()
	}
}

func (al *AgentLoop) newAsyncToolFollowUpCallback(
	ts *turnState,
	source string,
	iteration int,
	toolName string,
	logPublication bool,
) tools.AsyncCallback {
	return func(_ context.Context, result *tools.ToolResult) {
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

		if logPublication {
			logger.InfoCF("agent", "Async tool completed, publishing result",
				map[string]any{
					"tool":        toolName,
					"content_len": len(content),
					"channel":     ts.channel,
				})
		}

		al.emitEvent(
			EventKindFollowUpQueued,
			ts.scope.meta(iteration, source, "turn.follow_up.queued"),
			FollowUpQueuedPayload{
				SourceTool: toolName,
				Channel:    ts.channel,
				ChatID:     ts.chatID,
				ContentLen: len(content),
			},
		)

		pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pubCancel()
		_ = al.bus.PublishInbound(pubCtx, bus.InboundMessage{
			Channel:  "system",
			SenderID: fmt.Sprintf("async:%s", toolName),
			ChatID:   fmt.Sprintf("%s:%s", ts.channel, ts.chatID),
			Content:  content,
		})
	}
}

func (al *AgentLoop) executeAgentTool(
	ctx context.Context,
	ts *turnState,
	toolName string,
	toolArgs map[string]any,
	asyncCallback tools.AsyncCallback,
) (*tools.ToolResult, time.Duration) {
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
	return toolResult, time.Since(toolStart)
}

func (al *AgentLoop) applyAfterToolHooks(
	ctx context.Context,
	ts *turnState,
	source string,
	toolName string,
	toolArgs map[string]any,
	toolResult *tools.ToolResult,
	toolDuration time.Duration,
) (afterToolHookOutcome, error) {
	outcome := afterToolHookOutcome{
		toolName:   toolName,
		toolResult: toolResult,
	}
	if al.hooks == nil {
		return outcome, nil
	}

	toolResp, decision := al.hooks.AfterTool(ctx, &ToolResultHookResponse{
		Meta:      ts.eventMeta(source, "turn.tool.after"),
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
				outcome.toolName = toolResp.Tool
			}
			if toolResp.Result != nil {
				outcome.toolResult = toolResp.Result
			}
		}
	case HookActionAbortTurn:
		return outcome, al.hookAbortError(ts, "after_tool", decision)
	case HookActionHardAbort:
		_ = ts.requestHardAbort()
		outcome.hardAbort = true
	}

	return outcome, nil
}

func (al *AgentLoop) finalizeToolExecution(
	ctx context.Context,
	ts *turnState,
	source string,
	toolCallID string,
	toolName string,
	toolResult *tools.ToolResult,
	toolDuration time.Duration,
) finalizedToolExecution {
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
		ts.eventMeta(source, "turn.tool.end"),
		ToolExecEndPayload{
			Tool:       toolName,
			Duration:   toolDuration,
			ForLLMLen:  len(contentForLLM),
			ForUserLen: len(toolResult.ForUser),
			IsError:    toolResult.IsError,
			Async:      toolResult.Async,
		},
	)

	al.persistToolMessage(ctx, ts, toolResultMsg, true)

	return finalizedToolExecution{
		toolResult:    toolResult,
		toolMessage:   toolResultMsg,
		contentForLLM: contentForLLM,
	}
}

func (al *AgentLoop) persistToolSkipped(
	ctx context.Context,
	ts *turnState,
	source string,
	toolCallID string,
	toolName string,
	content string,
	ingest bool,
) providers.Message {
	al.emitEvent(
		EventKindToolExecSkipped,
		ts.eventMeta(source, "turn.tool.skipped"),
		ToolExecSkippedPayload{
			Tool:   toolName,
			Reason: content,
		},
	)

	toolMsg := providers.Message{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}
	al.persistToolMessage(ctx, ts, toolMsg, ingest)
	return toolMsg
}

func (al *AgentLoop) persistToolMessage(
	ctx context.Context,
	ts *turnState,
	msg providers.Message,
	ingest bool,
) {
	if ts.opts.NoHistory {
		return
	}
	ts.agent.Sessions.AddFullMessage(ts.sessionKey, msg)
	ts.recordPersistedMessage(msg)
	if ingest {
		ts.ingestMessage(ctx, al, msg)
	}
}
