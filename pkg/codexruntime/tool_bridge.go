package codexruntime

import (
	"context"
	"fmt"
)

type ToolCallRequest struct {
	CallID    string
	Name      string
	Arguments map[string]any
}

type ToolCallResult = ToolCallResponse

type ToolCallHandler func(context.Context, ToolCallRequest) (ToolCallResult, error)

func handleToolCall(ctx context.Context, req ToolCallRequest, handler ToolCallHandler) (ToolCallResponse, error) {
	if handler == nil {
		return toolFailure(fmt.Sprintf("unknown tool: %s", req.Name)), nil
	}

	result, err := handler(ctx, req)
	if err != nil {
		return toolFailure(err.Error()), nil
	}
	if len(result.Content) == 0 {
		result.Content = []ToolResultContentItem{{Type: "text"}}
	}

	return ToolCallResponse{
		Content: result.Content,
		Success: result.Success,
	}, nil
}

func toolFailure(message string) ToolCallResponse {
	return ToolCallResponse{
		Content: []ToolResultContentItem{{
			Type: "text",
			Text: message,
		}},
		Success: false,
	}
}
