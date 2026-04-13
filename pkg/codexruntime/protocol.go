package codexruntime

import "encoding/json"

const (
	MethodInitialize                          = "initialize"
	MethodInitialized                         = "initialized"
	MethodModelList                           = "model/list"
	MethodThreadStart                         = "thread/start"
	MethodThreadCompactStart                  = "thread/compact/start"
	MethodThreadCompacted                     = "thread/compacted"
	MethodThreadResume                        = "thread/resume"
	MethodTurnStart                           = "turn/start"
	MethodItemToolCall                        = "item/tool/call"
	MethodItemCommandExecutionRequestApproval = "item/commandExecution/requestApproval"
	MethodItemFileChangeRequestApproval       = "item/fileChange/requestApproval"
	MethodItemPermissionsRequestApproval      = "item/permissions/requestApproval"
	MethodItemAgentMessageDelta               = "item/agentMessage/delta"
	MethodItemReasoningTextDelta              = "item/reasoning/textDelta"
	MethodItemCompleted                       = "item/completed"

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
	ThreadID string `json:"thread_id"`
}

type ThreadResumeParams struct {
	ThreadID       string                  `json:"thread_id"`
	DynamicTools   []DynamicToolDefinition `json:"dynamicTools,omitempty"`
	ApprovalPolicy string                  `json:"approvalPolicy,omitempty"`
}

type ThreadCompactStartParams struct {
	ThreadID string `json:"thread_id"`
}

type TurnStartParams struct {
	ThreadID       string `json:"thread_id"`
	InputText      string `json:"input_text"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
}

type TurnStartResult struct {
	Content string `json:"content"`
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
