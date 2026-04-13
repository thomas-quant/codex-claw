package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thomas-quant/codex-claw/pkg/providers"
	"github.com/thomas-quant/codex-claw/pkg/tools"
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

	before, err := al.applyBeforeToolHooks(ctx, ts, "runInteractiveTool", toolName, toolArgs, false)
	if err != nil {
		return providers.InteractiveToolResult{}, err
	}
	if before.hardAbort {
		return providers.InteractiveToolResult{}, context.Canceled
	}
	toolName = before.toolName
	toolArgs = before.toolArgs

	if before.hookResult != nil {
		finalized := al.finalizeToolExecution(ctx, ts, "runInteractiveTool", call.CallID, toolName, before.hookResult, 0)
		return finalized.interactiveResult(), nil
	}

	if before.denyContent != "" {
		deniedMsg := al.persistToolSkipped(ctx, ts, "runInteractiveTool", call.CallID, toolName, before.denyContent, true)
		return providers.InteractiveToolResult{
			Success: false,
			ContentItems: []providers.InteractiveContentItem{
				{Type: "text", Text: deniedMsg.Content},
			},
		}, nil
	}

	if denyContent := al.approveToolExecution(ctx, ts, "runInteractiveTool", toolName, toolArgs); denyContent != "" {
		deniedMsg := al.persistToolSkipped(ctx, ts, "runInteractiveTool", call.CallID, toolName, denyContent, true)
		return providers.InteractiveToolResult{
			Success: false,
			ContentItems: []providers.InteractiveContentItem{
				{Type: "text", Text: deniedMsg.Content},
			},
		}, nil
	}

	al.emitToolExecutionStart(ctx, ts, "runInteractiveTool", "Interactive tool call", toolName, toolName, toolArgs)
	toolResult, toolDuration := al.executeAgentTool(
		ctx,
		ts,
		toolName,
		toolArgs,
		al.newAsyncToolFollowUpCallback(ts, "runInteractiveTool", iteration, toolName, false),
	)

	after, err := al.applyAfterToolHooks(ctx, ts, "runInteractiveTool", toolName, toolArgs, toolResult, toolDuration)
	if err != nil {
		return providers.InteractiveToolResult{}, err
	}
	if after.hardAbort {
		return providers.InteractiveToolResult{}, context.Canceled
	}
	toolName = after.toolName
	toolResult = after.toolResult
	if toolResult == nil {
		toolResult = tools.ErrorResult("hook returned nil tool result")
	}

	finalized := al.finalizeToolExecution(ctx, ts, "runInteractiveTool", call.CallID, toolName, toolResult, toolDuration)
	return finalized.interactiveResult(), nil
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
		al.persistToolMessage(ctx, ts, assistantMsg, true)
	}
}
