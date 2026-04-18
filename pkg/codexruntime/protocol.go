package codexruntime

import (
	"encoding/json"
	"time"
)

const (
	MethodInitialize                          = "initialize"
	MethodInitialized                         = "initialized"
	MethodModelList                           = "model/list"
	MethodAccountRead                         = "account/read"
	MethodAccountRateLimitsRead               = "account/rateLimits/read"
	MethodThreadStart                         = "thread/start"
	MethodThreadCompactStart                  = "thread/compact/start"
	MethodThreadCompacted                     = "thread/compacted"
	MethodThreadResume                        = "thread/resume"
	MethodTurnStart                           = "turn/start"
	MethodTurnSteer                           = "turn/steer"
	MethodItemToolCall                        = "item/tool/call"
	MethodItemToolRequestUserInput            = "item/tool/requestUserInput"
	MethodItemCommandExecutionRequestApproval = "item/commandExecution/requestApproval"
	MethodItemFileChangeRequestApproval       = "item/fileChange/requestApproval"
	MethodItemPermissionsRequestApproval      = "item/permissions/requestApproval"
	MethodItemAgentMessageDelta               = "item/agentMessage/delta"
	MethodItemReasoningTextDelta              = "item/reasoning/textDelta"
	MethodItemCompleted                       = "item/completed"
	MethodAccountChatgptAuthTokensRefresh     = "account/chatgptAuthTokens/refresh"

	ItemTypeAgentMessage      = "agent_message"
	ItemTypeContextCompaction = "contextCompaction"
	ItemTypeReasoning         = "reasoning"

	ItemRoleAssistant = "assistant"

	ItemStatusCompleted = "completed"
)

type Request struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type InitializeParams struct {
	ClientInfo map[string]any `json:"clientInfo,omitempty"`
}

type AccountReadParams struct {
	RefreshToken bool `json:"refreshToken,omitempty"`
}

type AccountSnapshot struct {
	Email      string    `json:"email,omitempty"`
	PlanType   string    `json:"plan_type,omitempty"`
	AuthMode   string    `json:"auth_mode,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

type RateLimitSnapshot struct {
	ID                   string    `json:"id,omitempty"`
	Name                 string    `json:"name,omitempty"`
	PlanType             string    `json:"plan_type,omitempty"`
	PrimaryUsedPercent   *int      `json:"primary_used_percent,omitempty"`
	SecondaryUsedPercent *int      `json:"secondary_used_percent,omitempty"`
	PrimaryResetAt       time.Time `json:"primary_reset_at,omitempty"`
	SecondaryResetAt     time.Time `json:"secondary_reset_at,omitempty"`
	ObservedAt           time.Time `json:"observed_at,omitempty"`
}

type ThreadStartParams struct {
	Model          string                  `json:"model,omitempty"`
	DynamicTools   []DynamicToolDefinition `json:"dynamicTools,omitempty"`
	ApprovalPolicy string                  `json:"approvalPolicy,omitempty"`
}

type ModelListParams struct {
	Hidden bool `json:"hidden,omitempty"`
}

type ModelCatalogEntry struct {
	ID                     string   `json:"id"`
	Label                  string   `json:"label,omitempty"`
	Hidden                 bool     `json:"hidden,omitempty"`
	ReasoningEffortOptions []string `json:"reasoningEffortOptions,omitempty"`
	SpeedTier              string   `json:"speedTier,omitempty"`
	UpgradeTo              string   `json:"upgradeTo,omitempty"`
}

type ModelListResult struct {
	Models []ModelCatalogEntry `json:"models"`
}

type ThreadStartResult struct {
	ThreadID string `json:"-"`
	Thread   struct {
		ID string `json:"id"`
	} `json:"thread,omitempty"`
}

type ThreadResumeParams struct {
	ThreadID       string                  `json:"threadId"`
	DynamicTools   []DynamicToolDefinition `json:"dynamicTools,omitempty"`
	ApprovalPolicy string                  `json:"approvalPolicy,omitempty"`
}

type ThreadCompactStartParams struct {
	ThreadID string `json:"thread_id"`
}

type TurnInputItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
	Path string `json:"path,omitempty"`
}

type SandboxPolicy struct {
	Type          string   `json:"type,omitempty"`
	WritableRoots []string `json:"writableRoots,omitempty"`
	NetworkAccess bool     `json:"networkAccess,omitempty"`
}

type TurnStartParams struct {
	ThreadID       string          `json:"threadId"`
	Input          []TurnInputItem `json:"input,omitempty"`
	ApprovalPolicy string          `json:"approvalPolicy,omitempty"`
	SandboxPolicy  *SandboxPolicy  `json:"sandboxPolicy,omitempty"`
}

type TurnStartResult struct {
	TurnID string `json:"-"`
	Turn   struct {
		ID string `json:"id"`
	} `json:"turn,omitempty"`
}

type TurnSteerParams struct {
	ThreadID       string          `json:"threadId"`
	Input          []TurnInputItem `json:"input,omitempty"`
	ExpectedTurnID string          `json:"expectedTurnId"`
}

type AgentMessageDeltaParams struct {
	ThreadID string `json:"thread_id"`
	TurnID   string `json:"turn_id"`
	ItemID   string `json:"item_id"`
	Delta    string `json:"delta"`
}

type ReasoningTextDeltaParams struct {
	ThreadID string `json:"thread_id"`
	TurnID   string `json:"turn_id"`
	ItemID   string `json:"item_id"`
	Text     string `json:"text"`
}

type OutputItem struct {
	ID     string `json:"id"`
	Type   string `json:"type,omitempty"`
	Role   string `json:"role,omitempty"`
	Status string `json:"status,omitempty"`
	Text   string `json:"text,omitempty"`
}

type ItemCompletedParams struct {
	ThreadID string     `json:"thread_id"`
	TurnID   string     `json:"turn_id"`
	Item     OutputItem `json:"item"`
}

type DynamicToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type ToolCallRequestParams struct {
	ThreadID  string         `json:"thread_id"`
	TurnID    string         `json:"turn_id"`
	CallID    string         `json:"call_id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type ToolResultContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type ToolCallResponse struct {
	Content []ToolResultContentItem `json:"contentItems"`
	Success bool                    `json:"success"`
}

type CommandExecutionApprovalRequestParams struct {
	ThreadID       string `json:"thread_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
}

type FileChangeApprovalRequestParams struct {
	ThreadID       string `json:"thread_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
}

type PermissionsApprovalRequestParams struct {
	ThreadID             string         `json:"thread_id,omitempty"`
	TurnID               string         `json:"turn_id,omitempty"`
	ConversationID       string         `json:"conversationId,omitempty"`
	RequestedPermissions map[string]any `json:"permissions,omitempty"`
}

type ToolRequestUserInputParams struct {
	ThreadID  string                         `json:"-"`
	TurnID    string                         `json:"-"`
	ItemID    string                         `json:"-"`
	Questions []ToolRequestUserInputQuestion `json:"questions,omitempty"`
}

type ToolRequestUserInputQuestion struct {
	Header   string                       `json:"header,omitempty"`
	ID       string                       `json:"id,omitempty"`
	Question string                       `json:"question,omitempty"`
	IsOther  bool                         `json:"isOther,omitempty"`
	IsSecret bool                         `json:"isSecret,omitempty"`
	Options  []ToolRequestUserInputOption `json:"options,omitempty"`
}

type ToolRequestUserInputOption struct {
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type ToolRequestUserInputResponse struct {
	Answers map[string]ToolRequestUserInputAnswer `json:"answers"`
}

type ToolRequestUserInputAnswer struct {
	Answers []string `json:"answers"`
}

type ChatgptAuthTokensRefreshParams struct {
	Reason            string `json:"reason,omitempty"`
	PreviousAccountID string `json:"previousAccountId,omitempty"`
}

type ChatgptAuthTokensRefreshResponse struct {
	AccessToken      string  `json:"accessToken"`
	ChatgptAccountID string  `json:"chatgptAccountId"`
	ChatgptPlanType  *string `json:"chatgptPlanType,omitempty"`
}

func (r *ThreadStartResult) UnmarshalJSON(data []byte) error {
	type rawThreadStartResult struct {
		ThreadIDLegacy string `json:"thread_id"`
		ThreadID       string `json:"threadId"`
		Thread         struct {
			ID string `json:"id"`
		} `json:"thread"`
	}

	var raw rawThreadStartResult
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy, raw.Thread.ID)
	r.Thread = raw.Thread
	return nil
}

func (r *TurnStartResult) UnmarshalJSON(data []byte) error {
	type rawTurnStartResult struct {
		TurnIDLegacy string `json:"turn_id"`
		TurnID       string `json:"turnId"`
		Turn         struct {
			ID string `json:"id"`
		} `json:"turn"`
	}

	var raw rawTurnStartResult
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy, raw.Turn.ID)
	r.Turn = raw.Turn
	return nil
}

func (p *AgentMessageDeltaParams) UnmarshalJSON(data []byte) error {
	type rawAgentMessageDeltaParams struct {
		ThreadIDLegacy string `json:"thread_id"`
		ThreadID       string `json:"threadId"`
		TurnIDLegacy   string `json:"turn_id"`
		TurnID         string `json:"turnId"`
		ItemIDLegacy   string `json:"item_id"`
		ItemID         string `json:"itemId"`
		Delta          string `json:"delta"`
	}

	var raw rawAgentMessageDeltaParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.ItemID = firstProtocolValue(raw.ItemID, raw.ItemIDLegacy)
	p.Delta = raw.Delta
	return nil
}

func (p *ReasoningTextDeltaParams) UnmarshalJSON(data []byte) error {
	type rawReasoningTextDeltaParams struct {
		ThreadIDLegacy string `json:"thread_id"`
		ThreadID       string `json:"threadId"`
		TurnIDLegacy   string `json:"turn_id"`
		TurnID         string `json:"turnId"`
		ItemIDLegacy   string `json:"item_id"`
		ItemID         string `json:"itemId"`
		Text           string `json:"text"`
	}

	var raw rawReasoningTextDeltaParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.ItemID = firstProtocolValue(raw.ItemID, raw.ItemIDLegacy)
	p.Text = raw.Text
	return nil
}

func (p *ItemCompletedParams) UnmarshalJSON(data []byte) error {
	type rawItemCompletedParams struct {
		ThreadIDLegacy string     `json:"thread_id"`
		ThreadID       string     `json:"threadId"`
		TurnIDLegacy   string     `json:"turn_id"`
		TurnID         string     `json:"turnId"`
		Item           OutputItem `json:"item"`
	}

	var raw rawItemCompletedParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.Item = raw.Item
	return nil
}

func (p *ToolCallRequestParams) UnmarshalJSON(data []byte) error {
	type rawToolCallRequestParams struct {
		ThreadIDLegacy string         `json:"thread_id"`
		ThreadID       string         `json:"threadId"`
		TurnIDLegacy   string         `json:"turn_id"`
		TurnID         string         `json:"turnId"`
		CallIDLegacy   string         `json:"call_id"`
		CallID         string         `json:"callId"`
		ItemID         string         `json:"itemId"`
		Name           string         `json:"name"`
		Tool           string         `json:"tool"`
		Arguments      map[string]any `json:"arguments"`
	}

	var raw rawToolCallRequestParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.CallID = firstProtocolValue(raw.CallID, raw.CallIDLegacy, raw.ItemID)
	p.Name = firstProtocolValue(raw.Name, raw.Tool)
	p.Arguments = raw.Arguments
	return nil
}

func (p *CommandExecutionApprovalRequestParams) UnmarshalJSON(data []byte) error {
	type rawCommandExecutionApprovalRequestParams struct {
		ThreadIDLegacy string `json:"thread_id"`
		ThreadID       string `json:"threadId"`
		TurnIDLegacy   string `json:"turn_id"`
		TurnID         string `json:"turnId"`
		ConversationID string `json:"conversationId"`
	}

	var raw rawCommandExecutionApprovalRequestParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.ConversationID = raw.ConversationID
	return nil
}

func (p *FileChangeApprovalRequestParams) UnmarshalJSON(data []byte) error {
	type rawFileChangeApprovalRequestParams struct {
		ThreadIDLegacy string `json:"thread_id"`
		ThreadID       string `json:"threadId"`
		TurnIDLegacy   string `json:"turn_id"`
		TurnID         string `json:"turnId"`
		ConversationID string `json:"conversationId"`
	}

	var raw rawFileChangeApprovalRequestParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.ConversationID = raw.ConversationID
	return nil
}

func (p *PermissionsApprovalRequestParams) UnmarshalJSON(data []byte) error {
	type rawPermissionsApprovalRequestParams struct {
		ThreadIDLegacy       string         `json:"thread_id"`
		ThreadID             string         `json:"threadId"`
		TurnIDLegacy         string         `json:"turn_id"`
		TurnID               string         `json:"turnId"`
		ConversationID       string         `json:"conversationId"`
		RequestedPermissions map[string]any `json:"permissions"`
	}

	var raw rawPermissionsApprovalRequestParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = firstProtocolValue(raw.ThreadID, raw.ThreadIDLegacy)
	p.TurnID = firstProtocolValue(raw.TurnID, raw.TurnIDLegacy)
	p.ConversationID = raw.ConversationID
	p.RequestedPermissions = raw.RequestedPermissions
	return nil
}

func (p *ToolRequestUserInputParams) UnmarshalJSON(data []byte) error {
	type rawToolRequestUserInputParams struct {
		ThreadID  string                         `json:"threadId"`
		TurnID    string                         `json:"turnId"`
		ItemID    string                         `json:"itemId"`
		Questions []ToolRequestUserInputQuestion `json:"questions"`
	}

	var raw rawToolRequestUserInputParams
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ThreadID = raw.ThreadID
	p.TurnID = raw.TurnID
	p.ItemID = raw.ItemID
	p.Questions = raw.Questions
	return nil
}

func firstProtocolValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
