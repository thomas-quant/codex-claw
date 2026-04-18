package codexruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var errClientNotStarted = errors.New("codexruntime: client not started")
var errClientTurnInProgress = errors.New("codexruntime: turn in progress")
var errClientNoActiveTurn = errors.New("codexruntime: no active turn")

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
	activeTurn *activeTurnState
	nextID     atomic.Int64
}

type RunTurnRequest struct {
	ThreadID       string
	InputText      string
	Input          []TurnInputItem
	SandboxPolicy  *SandboxPolicy
	HandleToolCall ToolCallHandler
	OnChunk        func(string)
}

type steerRequest struct {
	threadID string
	input    []TurnInputItem
	resultCh chan error
}

type activeTurnState struct {
	ThreadID string
	TurnID   string
	SteerCh  chan steerRequest
	ReadCancel context.CancelFunc
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
		"capabilities": map[string]any{
			"experimentalApi": true,
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

func (c *Client) ReadAccount(ctx context.Context, refreshToken bool) (AccountSnapshot, error) {
	var raw map[string]any
	if err := c.Call(ctx, MethodAccountRead, AccountReadParams{RefreshToken: refreshToken}, &raw); err != nil {
		return AccountSnapshot{}, err
	}

	return parseAccountReadResult(raw, time.Now().UTC())
}

func (c *Client) ReadRateLimits(ctx context.Context) ([]RateLimitSnapshot, error) {
	var raw map[string]any
	if err := c.Call(ctx, MethodAccountRateLimitsRead, map[string]any{}, &raw); err != nil {
		return nil, err
	}

	return parseRateLimitsResult(raw, time.Now().UTC())
}

func (c *Client) ListModels(ctx context.Context) ([]ModelCatalogEntry, error) {
	var result ModelListResult
	if err := c.Call(ctx, MethodModelList, ModelListParams{}, &result); err != nil {
		return nil, err
	}

	return append([]ModelCatalogEntry(nil), result.Models...), nil
}

func (c *Client) StartThread(ctx context.Context, model string, dynamicTools []DynamicToolDefinition) (string, error) {
	var result ThreadStartResult

	if err := c.Call(ctx, MethodThreadStart, ThreadStartParams{
		Model:          model,
		DynamicTools:   dynamicTools,
		ApprovalPolicy: approvalPolicyPermanentYOLO,
	}, &result); err != nil {
		return "", err
	}

	threadID := result.ThreadID
	if threadID == "" {
		return "", fmt.Errorf("codexruntime: thread/start returned empty thread_id")
	}

	return threadID, nil
}

func (c *Client) StartNativeCompaction(ctx context.Context, threadID string) error {
	return c.Call(ctx, MethodThreadCompactStart, ThreadCompactStartParams{ThreadID: threadID}, nil)
}

func (c *Client) SteerTurn(ctx context.Context, threadID string, input []TurnInputItem) error {
	active := c.activeTurnState()
	waitFor := c.opts.RequestTimeout
	if waitFor <= 0 {
		waitFor = time.Second
	}
	deadline := time.NewTimer(waitFor)
	ticker := time.NewTicker(time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for active == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errClientNoActiveTurn
		case <-ticker.C:
		}
		active = c.activeTurnState()
	}
	if active.ThreadID != threadID {
		return fmt.Errorf("codexruntime: active turn thread mismatch: got %s want %s", threadID, active.ThreadID)
	}

	req := steerRequest{
		threadID: threadID,
		input:    append([]TurnInputItem(nil), input...),
		resultCh: make(chan error, 1),
	}

	select {
	case active.SteerCh <- req:
		c.wakeActiveTurnReader()
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-req.resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
	var result TurnStartResult

	c.ioMu.Lock()

	if c.isTurnActiveLocked() {
		c.ioMu.Unlock()
		return "", errClientTurnInProgress
	}
	if !c.hasConnLocked() {
		c.ioMu.Unlock()
		return "", errClientNotStarted
	}
	active := &activeTurnState{
		ThreadID: req.ThreadID,
		SteerCh:  make(chan steerRequest, 1),
	}
	c.setActiveTurnLocked(active)
	defer c.clearActiveTurn()

	if err := c.callLocked(ctx, c.nextRequestID(), MethodTurnStart, TurnStartParams{
		ThreadID:       req.ThreadID,
		Input:          buildTurnInput(req.Input, req.InputText),
		ApprovalPolicy: approvalPolicyPermanentYOLO,
		SandboxPolicy:  req.SandboxPolicy,
	}, &result); err != nil {
		c.ioMu.Unlock()
		return "", err
	}
	turnID := result.TurnID
	if turnID == "" {
		c.ioMu.Unlock()
		return "", fmt.Errorf("codexruntime: turn/start returned empty turn_id")
	}
	active.TurnID = turnID
	if err := c.drainPendingSteerLocked(ctx, req); err != nil {
		c.ioMu.Unlock()
		return "", err
	}
	c.ioMu.Unlock()

	projector := NewProjector(req.ThreadID, turnID)
	for {
		c.ioMu.Lock()
		if err := c.drainPendingSteerLocked(ctx, req); err != nil {
			c.ioMu.Unlock()
			return "", err
		}
		readCtx, cancelRead := context.WithCancel(ctx)
		c.setActiveReadCancel(cancelRead)
		message, err := c.readMessageLocked(readCtx)
		c.clearActiveReadCancel()
		cancelRead()
		if err == nil && isServerRequest(message) {
			err = c.routeServerRequestLocked(ctx, req, turnID, message)
		}
		c.ioMu.Unlock()
		if err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				continue
			}
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
				if err := params.err(); err != nil {
					return "", err
				}
				return projector.FinalAssistantText(), nil
			}
		}
	}

}

func buildTurnInput(input []TurnInputItem, inputText string) []TurnInputItem {
	if len(input) > 0 {
		return append([]TurnInputItem(nil), input...)
	}

	return []TurnInputItem{{Type: "text", Text: inputText}}
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
	ThreadID string `json:"-"`
	TurnID   string `json:"-"`
	Status   string `json:"-"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"-"`
}

func (p *turnCompletedParams) UnmarshalJSON(data []byte) error {
	type rawTurnCompletedParams struct {
		ThreadIDLegacy string `json:"thread_id"`
		ThreadID       string `json:"threadId"`
		TurnIDLegacy   string `json:"turn_id"`
		TurnID         string `json:"turnId"`
		Status         string `json:"status"`
		Error          *struct {
			Message string `json:"message"`
		} `json:"error"`
		Turn *struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}

	var raw rawTurnCompletedParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	if raw.Turn != nil {
		p.TurnID = firstProtocolValue(raw.Turn.ID, raw.TurnID, raw.TurnIDLegacy)
		p.Status = firstProtocolValue(raw.Turn.Status, raw.Status)
		if raw.Turn.Error != nil {
			p.Error = raw.Turn.Error
		} else {
			p.Error = raw.Error
		}
		return nil
	}

	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.Status = raw.Status
	p.Error = raw.Error
	return nil
}

func (p turnCompletedParams) err() error {
	switch p.Status {
	case "", "completed":
		return nil
	case "failed":
		if p.Error != nil && p.Error.Message != "" {
			return fmt.Errorf("codexruntime: turn failed: %s", p.Error.Message)
		}
		return fmt.Errorf("codexruntime: turn failed")
	case "interrupted":
		if p.Error != nil && p.Error.Message != "" {
			return fmt.Errorf("codexruntime: turn interrupted: %s", p.Error.Message)
		}
		return fmt.Errorf("codexruntime: turn interrupted")
	default:
		if p.Error != nil && p.Error.Message != "" {
			return fmt.Errorf("codexruntime: turn %s: %s", p.Status, p.Error.Message)
		}
		return nil
	}
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

func (c *Client) callDuringTurnLocked(
	ctx context.Context,
	id int64,
	method string,
	params any,
	result any,
	req RunTurnRequest,
	turnID string,
) error {
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

	for {
		message, err := c.readMessageLocked(callCtx)
		if err != nil {
			return err
		}
		if message.Method == "" {
			envelope := responseEnvelope{
				JSONRPC: message.JSONRPC,
				ID:      message.ID,
				Result:  message.Result,
				Error:   message.Error,
			}
			return decodeCallResult(envelope, id, method, result)
		}
		if isServerRequest(message) {
			if err := c.routeServerRequestLocked(callCtx, req, turnID, message); err != nil {
				return err
			}
			continue
		}

		c.pending = append(c.pending, message)
	}
}

func decodeCallResult(envelope responseEnvelope, id int64, method string, result any) error {
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
	case MethodItemToolRequestUserInput:
		var params ToolRequestUserInputParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		if params.ThreadID != req.ThreadID || params.TurnID != turnID {
			return c.rejectServerRequestLocked(ctx, message, -32600, "request thread/turn mismatch")
		}
		return c.respondToServerRequestLocked(ctx, message, buildToolRequestUserInputResponse(params))
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
	case MethodAccountChatgptAuthTokensRefresh:
		var params ChatgptAuthTokensRefreshParams
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return fmt.Errorf("codexruntime: decode %s params: %w", message.Method, err)
		}
		result, err := readChatgptAuthTokensRefreshResponse()
		if err != nil {
			return c.rejectServerRequestLocked(ctx, message, -32000, err.Error())
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

func (c *Client) drainPendingSteerLocked(ctx context.Context, req RunTurnRequest) error {
	active := c.activeTurnState()
	if active == nil {
		return nil
	}

	select {
	case steer := <-active.SteerCh:
		err := c.handleSteerRequestLocked(ctx, req, active, steer)
		steer.resultCh <- err
		return err
	default:
		return nil
	}
}

func (c *Client) handleSteerRequestLocked(
	ctx context.Context,
	req RunTurnRequest,
	active *activeTurnState,
	steer steerRequest,
) error {
	if active.ThreadID != steer.threadID {
		return fmt.Errorf("codexruntime: active turn thread mismatch: got %s want %s", steer.threadID, active.ThreadID)
	}

	return c.callDuringTurnLocked(ctx, c.nextRequestID(), MethodTurnSteer, TurnSteerParams{
		ThreadID:       steer.threadID,
		Input:          append([]TurnInputItem(nil), steer.input...),
		ExpectedTurnID: active.TurnID,
	}, nil, req, active.TurnID)
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

func (c *Client) isTurnActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.turnActive
}

func (c *Client) activeTurnState() *activeTurnState {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.activeTurn
}

func (c *Client) clearActiveTurn() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.activeTurn != nil {
		c.activeTurn.ReadCancel = nil
	}
	c.activeTurn = nil
	c.turnActive = false
}

func (c *Client) setActiveTurnLocked(active *activeTurnState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.activeTurn = active
	c.turnActive = active != nil
}

func (c *Client) setActiveReadCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.activeTurn != nil {
		c.activeTurn.ReadCancel = cancel
	}
}

func (c *Client) clearActiveReadCancel() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.activeTurn != nil {
		c.activeTurn.ReadCancel = nil
	}
}

func (c *Client) wakeActiveTurnReader() {
	c.mu.Lock()
	cancel := context.CancelFunc(nil)
	if c.activeTurn != nil {
		cancel = c.activeTurn.ReadCancel
	}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (c *Client) setTurnActiveLocked(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.turnActive = active
	if !active {
		c.activeTurn = nil
	}
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

func buildToolRequestUserInputResponse(params ToolRequestUserInputParams) ToolRequestUserInputResponse {
	answers := make(map[string]ToolRequestUserInputAnswer, len(params.Questions))
	for _, question := range params.Questions {
		answer := ToolRequestUserInputAnswer{Answers: []string{}}
		if question.ID == "" {
			continue
		}
		if len(question.Options) > 0 && question.Options[0].Label != "" {
			answer.Answers = []string{question.Options[0].Label}
		}
		answers[question.ID] = answer
	}
	return ToolRequestUserInputResponse{Answers: answers}
}

func readChatgptAuthTokensRefreshResponse() (ChatgptAuthTokensRefreshResponse, error) {
	authPath, err := currentCodexAuthPath()
	if err != nil {
		return ChatgptAuthTokensRefreshResponse{}, err
	}

	raw, err := os.ReadFile(authPath)
	if err != nil {
		return ChatgptAuthTokensRefreshResponse{}, err
	}

	var payload struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ChatgptAuthTokensRefreshResponse{}, err
	}
	if payload.Tokens.AccessToken == "" || payload.Tokens.AccountID == "" {
		return ChatgptAuthTokensRefreshResponse{}, fmt.Errorf("chatgpt auth tokens unavailable")
	}

	return ChatgptAuthTokensRefreshResponse{
		AccessToken:      payload.Tokens.AccessToken,
		ChatgptAccountID: payload.Tokens.AccountID,
	}, nil
}

func currentCodexAuthPath() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "auth.json"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("codexruntime: unable to resolve codex home")
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
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
