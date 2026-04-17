package codexruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClient_InitializeHandshake(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := transport.Methods(); !slices.Equal(got, []string{"initialize", "initialized"}) {
		t.Fatalf("Methods() = %v, want %v", got, []string{"initialize", "initialized"})
	}
}

func TestClient_InitializeHandshake_EnablesExperimentalAPI(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.120.0"}}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	writes := transport.Writes()
	if len(writes) < 1 {
		t.Fatalf("Writes() len = %d, want at least 1 initialize request", len(writes))
	}

	var envelope struct {
		Method string `json:"method"`
		Params struct {
			Capabilities struct {
				ExperimentalAPI bool `json:"experimentalApi"`
			} `json:"capabilities"`
		} `json:"params"`
	}
	if err := json.Unmarshal(writes[0], &envelope); err != nil {
		t.Fatalf("decode initialize request: %v", err)
	}
	if envelope.Method != MethodInitialize {
		t.Fatalf("initialize method = %q, want %q", envelope.Method, MethodInitialize)
	}
	if !envelope.Params.Capabilities.ExperimentalAPI {
		t.Fatal("initialize experimentalApi = false, want true")
	}
}

func TestClient_CallDecodesResult(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"thread_id":"thr_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var got struct {
		ThreadID string `json:"thread_id"`
	}
	if err := client.Call(context.Background(), "thread/start", map[string]any{"model": "gpt-5.4"}, &got); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if got.ThreadID != "thr_123" {
		t.Fatalf("Call() decoded thread_id = %q, want %q", got.ThreadID, "thr_123")
	}
	if got := transport.Methods(); !slices.Equal(got, []string{"initialize", "initialized", "thread/start"}) {
		t.Fatalf("Methods() = %v, want %v", got, []string{"initialize", "initialized", "thread/start"})
	}
}

func TestClient_StartThreadAcceptsAppServerShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "thread object",
			response: `{"jsonrpc":"2.0","id":2,"result":{"thread":{"id":"thr_obj"}}}`,
			want:     "thr_obj",
		},
		{
			name:     "thread_id field",
			response: `{"jsonrpc":"2.0","id":2,"result":{"thread_id":"thr_field"}}`,
			want:     "thr_field",
		},
		{
			name:     "threadId field",
			response: `{"jsonrpc":"2.0","id":2,"result":{"threadId":"thr_camel"}}`,
			want:     "thr_camel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			transport := newScriptedTransport(
				`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
				tt.response,
			)
			client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

			if err := client.Start(context.Background()); err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			threadID, err := client.StartThread(context.Background(), "gpt-5.4", nil)
			if err != nil {
				t.Fatalf("StartThread() error = %v", err)
			}
			if threadID != tt.want {
				t.Fatalf("StartThread() = %q, want %q", threadID, tt.want)
			}
		})
	}
}

func TestClient_StartThreadUsesCurrentAppServerRequestShape(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"threadId":"thr_current"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	threadID, err := client.StartThread(context.Background(), "gpt-5.4", []DynamicToolDefinition{
		{
			Name:        "lookup_weather",
			Description: "Looks up weather",
			InputSchema: map[string]any{"type": "object"},
		},
	})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if threadID != "thr_current" {
		t.Fatalf("StartThread() = %q, want %q", threadID, "thr_current")
	}

	writes := transport.Writes()
	if len(writes) != 3 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 3)
	}

	var envelope struct {
		Method string `json:"method"`
		Params struct {
			Model          string `json:"model"`
			ApprovalPolicy string `json:"approvalPolicy"`
			DynamicTools   []struct {
				Name string `json:"name"`
			} `json:"dynamicTools"`
		} `json:"params"`
	}
	if err := json.Unmarshal(writes[2], &envelope); err != nil {
		t.Fatalf("decode thread/start request: %v", err)
	}
	if envelope.Method != MethodThreadStart {
		t.Fatalf("thread/start method = %q, want %q", envelope.Method, MethodThreadStart)
	}
	if envelope.Params.Model != "gpt-5.4" {
		t.Fatalf("thread/start model = %q, want %q", envelope.Params.Model, "gpt-5.4")
	}
	if envelope.Params.ApprovalPolicy != approvalPolicyPermanentYOLO {
		t.Fatalf("thread/start approvalPolicy = %q, want %q", envelope.Params.ApprovalPolicy, approvalPolicyPermanentYOLO)
	}
	if len(envelope.Params.DynamicTools) != 1 || envelope.Params.DynamicTools[0].Name != "lookup_weather" {
		t.Fatalf("thread/start dynamicTools = %#v, want lookup_weather entry", envelope.Params.DynamicTools)
	}
}

func TestClient_ListModelsDecodesCatalogEntries(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"models":[{"id":"gpt-5.4","label":"GPT-5.4","reasoningEffortOptions":["minimal","medium","high"],"speedTier":"standard"},{"id":"gpt-5.4-mini","label":"GPT-5.4 mini","reasoningEffortOptions":["minimal"],"speedTier":"fast","hidden":true}]}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("ListModels() len = %d, want %d", len(models), 2)
	}
	if models[0].ID != "gpt-5.4" || models[0].Label != "GPT-5.4" {
		t.Fatalf("ListModels()[0] = %#v, want gpt-5.4 catalog entry", models[0])
	}
	if !slices.Equal(models[0].ReasoningEffortOptions, []string{"minimal", "medium", "high"}) {
		t.Fatalf("ListModels()[0].ReasoningEffortOptions = %v, want %v", models[0].ReasoningEffortOptions, []string{"minimal", "medium", "high"})
	}
	if models[1].SpeedTier != "fast" || !models[1].Hidden {
		t.Fatalf("ListModels()[1] = %#v, want fast hidden entry", models[1])
	}
	if got := transport.Methods(); !slices.Equal(got, []string{"initialize", "initialized", MethodModelList}) {
		t.Fatalf("Methods() = %v, want %v", got, []string{"initialize", "initialized", MethodModelList})
	}
}

func TestClient_StartNativeCompactionUsesThreadCompactStart(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := client.StartNativeCompaction(context.Background(), "thr_123"); err != nil {
		t.Fatalf("StartNativeCompaction() error = %v", err)
	}

	writes := transport.Writes()
	if len(writes) != 3 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 3)
	}

	var envelope struct {
		Method string `json:"method"`
		Params struct {
			ThreadID string `json:"thread_id"`
		} `json:"params"`
	}
	if err := json.Unmarshal(writes[2], &envelope); err != nil {
		t.Fatalf("decode compact request: %v", err)
	}
	if envelope.Method != MethodThreadCompactStart {
		t.Fatalf("compact method = %q, want %q", envelope.Method, MethodThreadCompactStart)
	}
	if envelope.Params.ThreadID != "thr_123" {
		t.Fatalf("compact thread_id = %q, want %q", envelope.Params.ThreadID, "thr_123")
	}
}

func TestClient_StartThreadSkipsNotificationBeforeResponse(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","method":"session/updated","params":{"state":"ready"}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"thread_id":"thr_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	threadID, err := client.StartThread(context.Background(), "gpt-5.4", nil)
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if threadID != "thr_123" {
		t.Fatalf("StartThread() = %q, want %q", threadID, "thr_123")
	}
}

func TestClient_StartThreadRejectsResponseWithoutID(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","result":{"thread_id":"thr_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err := client.StartThread(context.Background(), "gpt-5.4", nil)
	if err == nil {
		t.Fatal("StartThread() error = nil, want malformed response error")
	}
	if !strings.Contains(err.Error(), "response id = 0") {
		t.Fatalf("StartThread() error = %v, want malformed response id error", err)
	}
}

func TestClient_StartThreadRejectsErrorResponseWithWrongID(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":999,"error":{"code":-32000,"message":"boom"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err := client.StartThread(context.Background(), "gpt-5.4", nil)
	if err == nil {
		t.Fatal("StartThread() error = nil, want response id mismatch")
	}
	if !strings.Contains(err.Error(), "response id = 999") {
		t.Fatalf("StartThread() error = %v, want response id mismatch", err)
	}
}

func TestClient_RunTextTurnProjectsFinalAssistantFromNotifications(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_1","delta":"Hello"}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_other","turn_id":"turn_123","item_id":"msg_1","delta":" ignored"}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_1","delta":" there"}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_2","delta":"Final"}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_2","delta":" answer"}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var chunks []string
	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
		OnChunk: func(chunk string) {
			chunks = append(chunks, chunk)
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if content != "Final answer" {
		t.Fatalf("RunTextTurn() content = %q, want %q", content, "Final answer")
	}
	if !slices.Equal(chunks, []string{"Hello", " there", "Final", " answer"}) {
		t.Fatalf("RunTextTurn() chunks = %v, want %v", chunks, []string{"Hello", " there", "Final", " answer"})
	}
	if got := transport.Methods(); !slices.Equal(got, []string{"initialize", "initialized", "turn/start"}) {
		t.Fatalf("Methods() = %v, want %v", got, []string{"initialize", "initialized", "turn/start"})
	}
}

func TestClient_RunTextTurnUsesCurrentAppServerTurnSchema(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turnId":"turn_123"}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"threadId":"thr_123","turnId":"turn_123","itemId":"msg_1","delta":"Hello"}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thr_123","turnId":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if content != "Hello" {
		t.Fatalf("RunTextTurn() content = %q, want %q", content, "Hello")
	}

	writes := transport.Writes()
	if len(writes) != 3 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 3)
	}

	var envelope struct {
		Method string `json:"method"`
		Params struct {
			ThreadIDLegacy string `json:"thread_id"`
			ThreadID       string `json:"threadId"`
			InputText      string `json:"input_text"`
			Input          []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"input"`
			ApprovalPolicy string `json:"approvalPolicy"`
		} `json:"params"`
	}
	if err := json.Unmarshal(writes[2], &envelope); err != nil {
		t.Fatalf("decode turn/start request: %v", err)
	}
	if envelope.Method != MethodTurnStart {
		t.Fatalf("turn/start method = %q, want %q", envelope.Method, MethodTurnStart)
	}
	if envelope.Params.ThreadID != "thr_123" {
		t.Fatalf("turn/start threadId = %q, want %q", envelope.Params.ThreadID, "thr_123")
	}
	if envelope.Params.ThreadIDLegacy != "" {
		t.Fatalf("turn/start thread_id = %q, want empty legacy field", envelope.Params.ThreadIDLegacy)
	}
	if envelope.Params.InputText != "" {
		t.Fatalf("turn/start input_text = %q, want empty legacy field", envelope.Params.InputText)
	}
	if len(envelope.Params.Input) != 1 {
		t.Fatalf("turn/start input len = %d, want %d", len(envelope.Params.Input), 1)
	}
	if envelope.Params.Input[0].Type != "text" || envelope.Params.Input[0].Text != "hi" {
		t.Fatalf("turn/start input = %#v, want one text input item", envelope.Params.Input)
	}
	if envelope.Params.ApprovalPolicy != approvalPolicyPermanentYOLO {
		t.Fatalf("turn/start approvalPolicy = %q, want %q", envelope.Params.ApprovalPolicy, approvalPolicyPermanentYOLO)
	}
}

func TestClient_RunTextTurnHandlesNestedTurnCompletedShape(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.120.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"threadId":"thr_123","turnId":"turn_123","itemId":"msg_1","delta":"OK"}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thr_123","turn":{"id":"turn_123","status":"completed","error":null}}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if content != "OK" {
		t.Fatalf("RunTextTurn() content = %q, want %q", content, "OK")
	}
}

func TestClient_RunTextTurnReturnsNestedTurnFailure(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.120.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thr_123","turn":{"id":"turn_123","status":"failed","error":{"message":"usage limit exceeded"}}}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err == nil {
		t.Fatal("RunTextTurn() error = nil, want turn failure")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "usage limit exceeded") {
		t.Fatalf("RunTextTurn() error = %v, want usage limit message", err)
	}
}

func TestClient_RunTextTurnReplaysBufferedNotificationsAfterResponse(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_1","delta":"Hello"}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_1","delta":" there"}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var chunks []string
	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
		OnChunk: func(chunk string) {
			chunks = append(chunks, chunk)
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	if content != "Hello there" {
		t.Fatalf("RunTextTurn() content = %q, want %q", content, "Hello there")
	}
	if !slices.Equal(chunks, []string{"Hello", " there"}) {
		t.Fatalf("RunTextTurn() chunks = %v, want %v", chunks, []string{"Hello", " there"})
	}
}

func TestClient_RunTextTurnHandlesToolCallServerRequest(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","id":99,"method":"item/tool/call","params":{"thread_id":"thr_123","turn_id":"turn_123","call_id":"call_1","name":"lookup_weather","arguments":{"city":"London"}}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
		HandleToolCall: func(_ context.Context, req ToolCallRequest) (ToolCallResult, error) {
			if req.CallID != "call_1" {
				t.Fatalf("tool call id = %q, want %q", req.CallID, "call_1")
			}
			if req.Name != "lookup_weather" {
				t.Fatalf("tool name = %q, want %q", req.Name, "lookup_weather")
			}
			return ToolCallResult{
				Success: true,
				Content: []ToolResultContentItem{{Type: "text", Text: "sunny"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if content != "" {
		t.Fatalf("RunTextTurn() content = %q, want empty content", content)
	}

	writes := transport.Writes()
	if len(writes) != 4 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 4)
	}

	var response struct {
		ID     int64            `json:"id"`
		Result ToolCallResponse `json:"result"`
	}
	if err := json.Unmarshal(writes[3], &response); err != nil {
		t.Fatalf("decode tool call response: %v", err)
	}
	if response.ID != 99 {
		t.Fatalf("tool call response id = %d, want %d", response.ID, 99)
	}
	if !response.Result.Success {
		t.Fatalf("tool call response success = %v, want true", response.Result.Success)
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Type != "inputText" {
		t.Fatalf("tool call response content type = %#v, want inputText item", response.Result.Content)
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Text != "sunny" {
		t.Fatalf("tool call response = %#v, want sunny text result", response.Result)
	}
}

func TestClient_RunTextTurnHandlesCurrentToolCallServerRequestShape(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.120.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","id":99,"method":"item/tool/call","params":{"threadId":"thr_123","turnId":"turn_123","itemId":"call_1","tool":"lookup_weather","arguments":{"city":"London"}}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thr_123","turnId":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
		HandleToolCall: func(_ context.Context, req ToolCallRequest) (ToolCallResult, error) {
			if req.CallID != "call_1" {
				t.Fatalf("tool call id = %q, want %q", req.CallID, "call_1")
			}
			if req.Name != "lookup_weather" {
				t.Fatalf("tool name = %q, want %q", req.Name, "lookup_weather")
			}
			return ToolCallResult{
				Success: true,
				Content: []ToolResultContentItem{{Type: "text", Text: "sunny"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if content != "" {
		t.Fatalf("RunTextTurn() content = %q, want empty content", content)
	}

	writes := transport.Writes()
	if len(writes) != 4 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 4)
	}

	var response struct {
		ID     int64            `json:"id"`
		Result ToolCallResponse `json:"result"`
	}
	if err := json.Unmarshal(writes[3], &response); err != nil {
		t.Fatalf("decode tool call response: %v", err)
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Type != "inputText" {
		t.Fatalf("tool call response content type = %#v, want inputText item", response.Result.Content)
	}
}

func TestClient_RunTextTurnHandlesPermissionsApprovalRequest(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","id":99,"method":"item/permissions/requestApproval","params":{"thread_id":"thr_123","turn_id":"turn_123","permissions":{"network":"enabled"}}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}

	writes := transport.Writes()
	if len(writes) != 4 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 4)
	}

	var response map[string]json.RawMessage
	if err := json.Unmarshal(writes[3], &response); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}
	if got := string(response["id"]); got != "99" {
		t.Fatalf("approval response id = %s, want 99", got)
	}
	if string(response["error"]) != "null" && len(response["error"]) != 0 {
		t.Fatalf("approval response error = %s, want empty", string(response["error"]))
	}

	var result map[string]any
	if err := json.Unmarshal(response["result"], &result); err != nil {
		t.Fatalf("decode approval result: %v", err)
	}
	if result["scope"] != "turn" {
		t.Fatalf("approval scope = %#v, want %q", result["scope"], "turn")
	}
}

func TestClient_RunTextTurnRejectsServerRequest(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","id":99,"method":"workspace/apply","params":{"path":"README.md"}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if content != "" {
		t.Fatalf("RunTextTurn() content = %q, want empty", content)
	}

	writes := transport.Writes()
	if len(writes) != 4 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 4)
	}

	var response struct {
		ID    int64          `json:"id"`
		Error *responseError `json:"error"`
	}
	if err := json.Unmarshal(writes[3], &response); err != nil {
		t.Fatalf("decode server request error response: %v", err)
	}
	if response.ID != 99 {
		t.Fatalf("server request error response id = %d, want %d", response.ID, 99)
	}
	if response.Error == nil || response.Error.Code != -32601 {
		t.Fatalf("server request error response = %#v, want JSON-RPC method-not-found error", response.Error)
	}
}

func TestClient_RunTextTurnRejectsServerRequestWithZeroOrStringID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		request   string
		wantRawID string
	}{
		{
			name:      "zero id",
			request:   `{"jsonrpc":"2.0","id":0,"method":"workspace/apply","params":{"path":"README.md"}}`,
			wantRawID: `0`,
		},
		{
			name:      "string id",
			request:   `{"jsonrpc":"2.0","id":"req-7","method":"workspace/apply","params":{"path":"README.md"}}`,
			wantRawID: `"req-7"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			transport := newScriptedTransport(
				`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
				`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
				tt.request,
				`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
			)
			client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

			if err := client.Start(context.Background()); err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
				ThreadID:  "thr_123",
				InputText: "hi",
			})
			if err != nil {
				t.Fatalf("RunTextTurn() error = %v", err)
			}
			if content != "" {
				t.Fatalf("RunTextTurn() content = %q, want empty", content)
			}

			writes := transport.Writes()
			if len(writes) != 4 {
				t.Fatalf("Writes() len = %d, want %d", len(writes), 4)
			}

			var response map[string]json.RawMessage
			if err := json.Unmarshal(writes[3], &response); err != nil {
				t.Fatalf("decode server request error response: %v", err)
			}
			if got := string(response["id"]); got != tt.wantRawID {
				t.Fatalf("server request error response id = %s, want %s", got, tt.wantRawID)
			}
		})
	}
}

func TestClient_RunTextTurnRejectsReentrantClientCallsDuringChunkCallback(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":"thr_123","turn_id":"turn_123","item_id":"msg_1","delta":"Hello"}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	callbackErrCh := make(chan error, 1)
	resultCh := make(chan struct {
		content string
		err     error
	}, 1)

	go func() {
		content, err := client.RunTextTurn(ctx, RunTurnRequest{
			ThreadID:  "thr_123",
			InputText: "hi",
			OnChunk: func(string) {
				callbackErrCh <- client.Notify(context.Background(), "client/ping", map[string]any{"ok": true})
			},
		})
		resultCh <- struct {
			content string
			err     error
		}{content: content, err: err}
	}()

	select {
	case callbackErr := <-callbackErrCh:
		if !errors.Is(callbackErr, errClientTurnInProgress) {
			t.Fatalf("callback Notify() error = %v, want %v", callbackErr, errClientTurnInProgress)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for callback error")
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("RunTextTurn() error = %v", result.err)
		}
		if result.content != "Hello" {
			t.Fatalf("RunTextTurn() content = %q, want %q", result.content, "Hello")
		}
	case <-ctx.Done():
		t.Fatal("RunTextTurn() timed out")
	}
}

func TestClient_RunTextTurnRejectsToolCallRequestWithThreadMismatch(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","id":99,"method":"item/tool/call","params":{"thread_id":"thr_other","turn_id":"turn_123","call_id":"call_1","name":"lookup_weather","arguments":{"city":"London"}}}`,
		`{"jsonrpc":"2.0","method":"turn/completed","params":{"thread_id":"thr_123","turn_id":"turn_123"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	content, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err != nil {
		t.Fatalf("RunTextTurn() error = %v", err)
	}
	if content != "" {
		t.Fatalf("RunTextTurn() content = %q, want empty", content)
	}

	writes := transport.Writes()
	if len(writes) != 4 {
		t.Fatalf("Writes() len = %d, want %d", len(writes), 4)
	}

	var response struct {
		ID    int64          `json:"id"`
		Error *responseError `json:"error"`
	}
	if err := json.Unmarshal(writes[3], &response); err != nil {
		t.Fatalf("decode mismatch error response: %v", err)
	}
	if response.ID != 99 {
		t.Fatalf("mismatch error response id = %d, want %d", response.ID, 99)
	}
	if response.Error == nil || response.Error.Code != -32600 {
		t.Fatalf("mismatch error response = %#v, want JSON-RPC invalid-request error", response.Error)
	}
}

func TestClient_RunTextTurnReturnsDecodeErrorForMalformedNotification(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransport(
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
		`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"thread_id":123,"turn_id":"turn_123","item_id":"msg_1","delta":"Hello"}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err == nil {
		t.Fatal("RunTextTurn() error = nil, want decode failure")
	}
	if !strings.Contains(err.Error(), "decode item/agentMessage/delta params") {
		t.Fatalf("RunTextTurn() error = %v, want notification decode error", err)
	}
}

func TestClient_RunTextTurnReturnsReadErrorWhenTransportDiesMidTurn(t *testing.T) {
	t.Parallel()

	transport := newScriptedTransportWithReadErr(
		io.EOF,
		`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"version":"0.118.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"result":{"turn":{"id":"turn_123"}}}`,
	)
	client := NewClient(transport, ClientOptions{RequestTimeout: time.Second})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err := client.RunTextTurn(context.Background(), RunTurnRequest{
		ThreadID:  "thr_123",
		InputText: "hi",
	})
	if err == nil {
		t.Fatal("RunTextTurn() error = nil, want transport read error")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("RunTextTurn() error = %v, want EOF", err)
	}
}

type scriptedTransport struct {
	mu      sync.Mutex
	conn    *scriptedConn
	started bool
}

func newScriptedTransport(responses ...string) *scriptedTransport {
	return newScriptedTransportWithReadErr(context.DeadlineExceeded, responses...)
}

func newScriptedTransportWithReadErr(readErr error, responses ...string) *scriptedTransport {
	items := make([][]byte, 0, len(responses))
	for _, response := range responses {
		items = append(items, []byte(response))
	}

	return &scriptedTransport{
		conn: &scriptedConn{
			reads:   items,
			readErr: readErr,
		},
	}
}

func (t *scriptedTransport) Start(context.Context) (Conn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.started = true
	return t.conn, nil
}

func (t *scriptedTransport) Methods() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.conn.methods()
}

func (t *scriptedTransport) Writes() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.conn.writesCopy()
}

type scriptedConn struct {
	mu      sync.Mutex
	reads   [][]byte
	writes  [][]byte
	readErr error
}

func (c *scriptedConn) Read(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.reads) == 0 {
		if c.readErr == nil {
			return nil, context.DeadlineExceeded
		}
		return nil, c.readErr
	}

	next := c.reads[0]
	c.reads = c.reads[1:]
	return next, nil
}

func (c *scriptedConn) Write(_ context.Context, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.writes = append(c.writes, append([]byte(nil), payload...))
	return nil
}

func (c *scriptedConn) Close() error {
	return nil
}

func (c *scriptedConn) methods() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	methods := make([]string, 0, len(c.writes))
	for _, write := range c.writes {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(write, &envelope); err != nil {
			continue
		}

		var method string
		if err := json.Unmarshal(envelope["method"], &method); err != nil {
			continue
		}
		methods = append(methods, method)
	}

	return methods
}

func (c *scriptedConn) writesCopy() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	writes := make([][]byte, 0, len(c.writes))
	for _, write := range c.writes {
		writes = append(writes, append([]byte(nil), write...))
	}

	return writes
}
