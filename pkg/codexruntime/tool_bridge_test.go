package codexruntime

import (
	"context"
	"errors"
	"testing"
)

func TestToolBridge(t *testing.T) {
	t.Parallel()

	t.Run("success forwards content items", func(t *testing.T) {
		t.Parallel()

		got, err := handleToolCall(context.Background(), ToolCallRequest{
			CallID: "call-1",
			Name:   "lookup_weather",
			Arguments: map[string]any{
				"city": "London",
			},
		}, func(_ context.Context, req ToolCallRequest) (ToolCallResult, error) {
			if req.Name != "lookup_weather" {
				t.Fatalf("tool name = %q, want %q", req.Name, "lookup_weather")
			}

			return ToolCallResult{
				Success: true,
				Content: []ToolResultContentItem{
					{Type: "text", Text: "sunny"},
					{Type: "image", ImageURL: "https://example.com/icon.png"},
				},
			}, nil
		})
		if err != nil {
			t.Fatalf("handleToolCall() error = %v", err)
		}
		if !got.Success {
			t.Fatalf("handleToolCall() success = %v, want true", got.Success)
		}
		if len(got.Content) != 2 {
			t.Fatalf("handleToolCall() content len = %d, want %d", len(got.Content), 2)
		}
		if got.Content[0].Type != "inputText" {
			t.Fatalf("first content item type = %q, want %q", got.Content[0].Type, "inputText")
		}
		if got.Content[0].Text != "sunny" {
			t.Fatalf("first content item = %#v, want sunny text", got.Content[0])
		}
		if got.Content[1].Type != "inputImage" {
			t.Fatalf("second content item type = %q, want %q", got.Content[1].Type, "inputImage")
		}
		if got.Content[1].ImageURL != "https://example.com/icon.png" {
			t.Fatalf("second content item = %#v, want image URL", got.Content[1])
		}
	})

	t.Run("unknown tool returns failure payload", func(t *testing.T) {
		t.Parallel()

		got, err := handleToolCall(context.Background(), ToolCallRequest{
			CallID: "call-1",
			Name:   "missing_tool",
		}, nil)
		if err != nil {
			t.Fatalf("handleToolCall() error = %v", err)
		}
		if got.Success {
			t.Fatalf("handleToolCall() success = %v, want false", got.Success)
		}
		if len(got.Content) != 1 || got.Content[0].Type != "inputText" {
			t.Fatalf("handleToolCall() content = %#v, want one inputText item", got.Content)
		}
	})

	t.Run("tool error returns failure payload", func(t *testing.T) {
		t.Parallel()

		got, err := handleToolCall(context.Background(), ToolCallRequest{
			CallID: "call-1",
			Name:   "lookup_weather",
		}, func(context.Context, ToolCallRequest) (ToolCallResult, error) {
			return ToolCallResult{}, errors.New("tool failed")
		})
		if err != nil {
			t.Fatalf("handleToolCall() error = %v", err)
		}
		if got.Success {
			t.Fatalf("handleToolCall() success = %v, want false", got.Success)
		}
		if len(got.Content) != 1 || got.Content[0].Type != "inputText" {
			t.Fatalf("handleToolCall() content type = %#v, want one inputText item", got.Content)
		}
		if len(got.Content) != 1 || got.Content[0].Text != "tool failed" {
			t.Fatalf("handleToolCall() content = %#v, want tool error text", got.Content)
		}
	})
}
