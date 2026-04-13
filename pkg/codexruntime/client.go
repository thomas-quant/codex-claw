package codexruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var errClientNotStarted = errors.New("codexruntime: client not started")
var errClientTurnInProgress = errors.New("codexruntime: turn in progress")

const methodTurnCompleted = "turn/completed"

type Transport interface {
	Start(context.Context) (Conn, error)
}

type Conn interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close() error
}

type ClientOptions struct {
	RequestTimeout time.Duration
	ClientName     string
	ClientVersion  string
}

type Client struct {
	transport Transport
	opts      ClientOptions

	mu         sync.Mutex
	ioMu       sync.Mutex
	conn       Conn
	pending    []messageEnvelope
	turnActive bool
	nextID     atomic.Int64
}

type RunTurnRequest struct {
	ThreadID       string
	InputText      string
	HandleToolCall ToolCallHandler
	OnChunk        func(string)
}

func NewClient(transport Transport, opts ClientOptions) *Client {
	if opts.ClientName == "" {
		opts.ClientName = "codex-claw"
	}
	if opts.ClientVersion == "" {
		opts.ClientVersion = "0.0.0"
	}

	return &Client{
		transport: transport,
		opts:      opts,
	}
}

func (c *Client) Start(ctx context.Context) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	if c.isTurnActiveLocked() {
		return errClientTurnInProgress
	}

	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return nil
	}

	conn, err := c.transport.Start(ctx)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	c.conn = conn
	c.pending = nil
	c.mu.Unlock()
	if err := c.callLocked(ctx, c.nextRequestID(), "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    c.opts.ClientName,
			"version": c.opts.ClientVersion,
		},
	}, nil); err != nil {
		_ = c.closeLocked()
		return err
	}

	if err := c.notifyLocked(ctx, "initialized", nil); err != nil {
		_ = c.closeLocked()
		return err
	}

	return nil
}

func (c *Client) Close() error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	if c.isTurnActiveLocked() {
		return errClientTurnInProgress
	}

	return c.closeLocked()
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	if c.isTurnActiveLocked() {
		return errClientTurnInProgress
	}

	if !c.hasConnLocked() {
		return errClientNotStarted
	}

	return c.callLocked(ctx, c.nextRequestID(), method, params, result)
}

func (c *Client) Notify(ctx context.Context, method string, params any) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	if c.isTurnActiveLocked() {
		return errClientTurnInProgress
	}

	if !c.hasConnLocked() {
		return errClientNotStarted
	}

	return c.notifyLocked(ctx, method, params)
}

func (c *Client) ResumeThread(ctx context.Context, threadID string, dynamicTools []DynamicToolDefinition) error {
	return c.Call(ctx, MethodThreadResume, ThreadResumeParams{
		ThreadID:       threadID,
		DynamicTools:   dynamicTools,
		ApprovalPolicy: approvalPolicyPermanentYOLO,
	}, nil)
}

func (c *Client) Restart(ctx context.Context) error {
	if err := c.Close(); err != nil && !errors.Is(err, errClientNotStarted) {
		return err
	}
	return c.Start(ctx)
}

func (c *Client) ListModels(ctx context.Context) ([]ModelCatalogEntry, error) {
	var result ModelListResult
	if err := c.Call(ctx, MethodModelList, ModelListParams{}, &result); err != nil {
		return nil, err
	}

	return append([]ModelCatalogEntry(nil), result.Models...), nil
}

func (c *Client) StartThread(ctx context.Context, model string, dynamicTools []DynamicToolDefinition) (string, error) {
	var result struct {
		ThreadID string `json:"thread_id"`
		Thread   struct {
			ID string `json:"id"`
		} `json:"thread"`
	}

	if err := c.Call(ctx, MethodThreadStart, ThreadStartParams{
		Model:          model,
		DynamicTools:   dynamicTools,
		ApprovalPolicy: approvalPolicyPermanentYOLO,
	}, &result); err != nil {
		return "", err
	}

	threadID := result.ThreadID
	if threadID == "" {
		threadID = result.Thread.ID
	}
	if threadID == "" {
		return "", fmt.Errorf("codexruntime: thread/start returned empty thread_id")
	}

	return threadID, nil
}

func (c *Client) StartNativeCompaction(ctx context.Context, threadID string) error {
	return c.Call(ctx, MethodThreadCompactStart, ThreadCompactStartParams{ThreadID: threadID}, nil)
}

func (c *Client) Status() ClientStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	return ClientStatus{
		Started:    c.conn != nil,
		TurnActive: c.turnActive,
	}
}

func (c *Client) RunTextTurn(ctx context.Context, req RunTurnRequest) (string, error) {
	var result struct {
		TurnID string `json:"turn_id"`
		Turn   struct {
			ID string `json:"id"`
		} `json:"turn"`
	}

	c.ioMu.Lock()

	if c.isTurnActiveLocked() {
		c.ioMu.Unlock()
		return "", errClientTurnInProgress
	}
	if !c.hasConnLocked() {
		c.ioMu.Unlock()
		return "", errClientNotStarted
	}
	c.setTurnActiveLocked(true)

	if err := c.callLocked(ctx, c.nextRequestID(), MethodTurnStart, TurnStartParams{
		ThreadID:       req.ThreadID,
		InputText:      req.InputText,
		ApprovalPolicy: approvalPolicyPermanentYOLO,
	}, &result); err != nil {
		c.setTurnActiveLocked(false)
		c.ioMu.Unlock()
		return "", err
	}
	c.ioMu.Unlock()
	defer c.setTurnActive(false)

	turnID := result.TurnID
	if turnID == "" {
		turnID = result.Turn.ID
	}
	if turnID == "" {
		return "", fmt.Errorf("codexruntime: turn/start returned empty turn_id")
	}

	projector := NewProjector(req.ThreadID, turnID)
	for {
		c.ioMu.Lock()
		message, err := c.readMessageLocked(ctx)
		if err == nil && isServerRequest(message) {
			err = c.routeServerRequestLocked(ctx, req, turnID, message)
		}
		c.ioMu.Unlock()
		if err != nil {
			return "", err
		}
		if message.Method == "" {
			if message.Error != nil {
				if message.Error.Message == "" {
					return "", fmt.Errorf("codexruntime: turn/start stream failed")
				}
				return "", fmt.Errorf("codexruntime: turn/start stream failed: %s", message.Error.Message)
			}
			continue
		}

		notification, err := decodeNotification(message)
		if err != nil {
			return "", err
		}

		switch params := notification.Params.(type) {
		case AgentMessageDeltaParams:
			if params.ThreadID != req.ThreadID || params.TurnID != turnID {
				continue
			}
			projector.Apply(notification)
			if req.OnChunk != nil {
				req.OnChunk(params.Delta)
			}
		case ItemCompletedParams:
			projector.Apply(notification)
		case ReasoningTextDeltaParams:
			projector.Apply(notification)
		case turnCompletedParams:
			if params.ThreadID == req.ThreadID && params.TurnID == turnID {
				return projector.FinalAssistantText(), nil
			}
		}
	}

}

type requestEnvelope struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type responseEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type messageEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type turnCompletedParams struct {
	ThreadID string `json:"thread_id"`
	TurnID   string `json:"turn_id"`
}

func (c *Client) callLocked(ctx context.Context, id int64, method string, params any, result any) error {
	callCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	if err := c.writeLocked(callCtx, requestEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		return err
	}

	envelope, err := c.readResponseLocked(callCtx, method)
	if err != nil {
		return err
	}

	if envelope.Error != nil {
		responseID, idErr := decodeNumericID(envelope.ID)
		if idErr != nil {
			return idErr
		}
		if responseID != id {
			return fmt.Errorf("codexruntime: %s response id = %d, want %d", method, responseID, id)
		}
		if envelope.Error.Message == "" {
			return fmt.Errorf("codexruntime: %s failed", method)
		}
		return fmt.Errorf("codexruntime: %s failed: %s", method, envelope.Error.Message)
	}
	responseID, err := decodeNumericID(envelope.ID)
	if err != nil {
		return err
	}
	if responseID != id {
		return fmt.Errorf("codexruntime: %s response id = %d, want %d", method, responseID, id)
	}
	if result == nil || len(envelope.Result) == 0 {
		return nil
	}

	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("codexruntime: decode %s result: %w", method, err)
	}

	return nil
}

func (c *Client) notifyLocked(ctx context.Context, method string, params any) error {
	notifyCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	return c.writeLocked(notifyCtx, requestEnvelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (c *Client) writeLocked(ctx context.Context, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("codexruntime: encode payload: %w", err)
	}

	data = append(data, '\n')
	conn, err := c.connLocked()
	if err != nil {
		return err
	}
	if err := conn.Write(ctx, data); err != nil {
		return err
	}

	return nil
}

func (c *Client) readResponseLocked(ctx context.Context, method string) (responseEnvelope, error) {
	for {
		envelope, err := c.readFromConnLocked(ctx)
		if err != nil {
			return responseEnvelope{}, err
		}
		if envelope.Method == "" {
			return responseEnvelope{
				JSONRPC: envelope.JSONRPC,
				ID:      envelope.ID,
				Result:  envelope.Result,
				Error:   envelope.Error,
			}, nil
		}
		if isServerRequest(envelope) {
			if err := c.rejectServerRequestLocked(ctx, envelope, -32601, "server-initiated requests are unsupported"); err != nil {
				return responseEnvelope{}, err
			}
			return responseEnvelope{}, fmt.Errorf("codexruntime: server request %s is unsupported during %s", envelope.Method, method)
		}

		c.pending = append(c.pending, envelope)
	}
}

func (c *Client) readMessageLocked(ctx context.Context) (messageEnvelope, error) {
	if len(c.pending) > 0 {
		message := c.pending[0]
		c.pending = c.pending[1:]
		return message, nil
	}

	return c.readFromConnLocked(ctx)
}

func (c *Client) readFromConnLocked(ctx context.Context) (messageEnvelope, error) {
	conn, err := c.connLocked()
	if err != nil {
		return messageEnvelope{}, err
	}

	data, err := conn.Read(ctx)
	if err != nil {
		return messageEnvelope{}, err
	}

	var envelope messageEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return messageEnvelope{}, fmt.Errorf("codexruntime: decode response: %w", err)
	}

	return envelope, nil
}

func (c *Client) routeServerRequestLocked(ctx context.Context, req RunTurnRequest, turnID string, message messageEnvelope) error {
	switch message.Method {
	case MethodItemToolCall:
		var params ToolCallRequestParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		if params.ThreadID != req.ThreadID || params.TurnID != turnID {
			return c.rejectServerRequestLocked(ctx, message, -32600, "request thread/turn mismatch")
		}

		result, err := handleToolCall(ctx, ToolCallRequest{
			CallID:    params.CallID,
			Name:      params.Name,
			Arguments: params.Arguments,
		}, req.HandleToolCall)
		if err != nil {
			return err
		}
		return c.respondToServerRequestLocked(ctx, message, result)
	case MethodItemCommandExecutionRequestApproval:
		var params CommandExecutionApprovalRequestParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		if params.ThreadID != req.ThreadID || params.TurnID != turnID {
			return c.rejectServerRequestLocked(ctx, message, -32600, "request thread/turn mismatch")
		}
		result, err := handleApprovalRequest(message.Method, params)
		if err != nil {
			return err
		}
		return c.respondToServerRequestLocked(ctx, message, result)
	case MethodItemFileChangeRequestApproval:
		var params FileChangeApprovalRequestParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		if params.ThreadID != req.ThreadID || params.TurnID != turnID {
			return c.rejectServerRequestLocked(ctx, message, -32600, "request thread/turn mismatch")
		}
		result, err := handleApprovalRequest(message.Method, params)
		if err != nil {
			return err
		}
		return c.respondToServerRequestLocked(ctx, message, result)
	case MethodItemPermissionsRequestApproval:
		var params PermissionsApprovalRequestParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		if params.ThreadID != req.ThreadID || params.TurnID != turnID {
			return c.rejectServerRequestLocked(ctx, message, -32600, "request thread/turn mismatch")
		}
		result, err := handleApprovalRequest(message.Method, params)
		if err != nil {
			return err
		}
		return c.respondToServerRequestLocked(ctx, message, result)
	default:
		return c.rejectServerRequestLocked(ctx, message, -32601, "server-initiated requests are unsupported")
	}
}

func (c *Client) respondToServerRequestLocked(ctx context.Context, message messageEnvelope, result any) error {
	rejectCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	return c.writeLocked(rejectCtx, responseEnvelope{
		JSONRPC: rpcVersion(message.JSONRPC),
		ID:      message.ID,
		Result:  mustMarshalRawMessage(result),
	})
}

func (c *Client) rejectServerRequestLocked(ctx context.Context, message messageEnvelope, code int, text string) error {
	rejectCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	if err := c.writeLocked(rejectCtx, responseEnvelope{
		JSONRPC: rpcVersion(message.JSONRPC),
		ID:      message.ID,
		Error: &responseError{
			Code:    code,
			Message: text,
		},
	}); err != nil {
		return fmt.Errorf("codexruntime: server request %s rejected: %w", message.Method, err)
	}

	return nil
}

func decodeNotification(message messageEnvelope) (Notification, error) {
	notification := Notification{
		JSONRPC: message.JSONRPC,
		Method:  message.Method,
	}

	switch message.Method {
	case MethodItemAgentMessageDelta:
		var params AgentMessageDeltaParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return Notification{}, fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		notification.Params = params
	case MethodItemReasoningTextDelta:
		var params ReasoningTextDeltaParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return Notification{}, fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		notification.Params = params
	case MethodItemCompleted:
		var params ItemCompletedParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return Notification{}, fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		notification.Params = params
	case methodTurnCompleted:
		var params turnCompletedParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return Notification{}, fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		notification.Params = params
	default:
		var params map[string]any
		if len(message.Params) > 0 {
			if err := json.Unmarshal(message.Params, &params); err != nil {
				return Notification{}, fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
			}
		}
		notification.Params = params
	}

	return notification, nil
}

func (c *Client) closeLocked() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	conn := c.conn
	c.conn = nil
	c.pending = nil
	return conn.Close()
}

func (c *Client) isTurnActiveLocked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.turnActive
}

func (c *Client) setTurnActive(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.turnActive = active
}

func (c *Client) setTurnActiveLocked(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.turnActive = active
}

func (c *Client) hasConnLocked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn != nil
}

func (c *Client) connLocked() (Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, errClientNotStarted
	}

	return c.conn, nil
}

func isServerRequest(message messageEnvelope) bool {
	return message.Method != "" && hasID(message.ID)
}

func rpcVersion(version string) string {
	if version != "" {
		return version
	}

	return "2.0"
}

func (c *Client) nextRequestID() int64 {
	return c.nextID.Add(1)
}

func hasID(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func mustMarshalRawMessage(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return data
}

func decodeNumericID(raw json.RawMessage) (int64, error) {
	if !hasID(raw) {
		return 0, fmt.Errorf("codexruntime: response id = 0, want numeric id")
	}

	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, fmt.Errorf("codexruntime: unsupported json-rpc id %s", string(raw))
	}

	return id, nil
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.opts.RequestTimeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, c.opts.RequestTimeout)
}
