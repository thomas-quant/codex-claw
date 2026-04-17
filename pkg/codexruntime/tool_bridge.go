package codexruntime

import (
	"context"
	"fmt"
	"strings"
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
		result.Content = []ToolResultContentItem{{Type: "inputText"}}
	}

	return ToolCallResponse{
		Content: normalizeToolResultContentItems(result.Content),
		Success: result.Success,
	}, nil
}

func toolFailure(message string) ToolCallResponse {
	return ToolCallResponse{
		Content: []ToolResultContentItem{{
			Type: "inputText",
			Text: message,
		}},
		Success: false,
	}
}

func normalizeToolResultContentItems(items []ToolResultContentItem) []ToolResultContentItem {
	normalized := make([]ToolResultContentItem, 0, len(items))
	for _, item := range items {
		switch strings.TrimSpace(item.Type) {
		case "", "text", "inputText":
			item.Type = "inputText"
		case "image", "imageUrl", "inputImage":
			item.Type = "inputImage"
		}
		normalized = append(normalized, item)
	}
	return normalized
}
