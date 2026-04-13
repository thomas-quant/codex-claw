## Events

Compressed technical summary of runtime notifications and approval handshakes.

### Event Stream Model

- After `thread/start` or `thread/resume`, clients should continuously read notifications:
  - Thread lifecycle: `thread/started`, `thread/archived`, `thread/unarchived`, `thread/closed`.
  - Turn lifecycle: `turn/started`, `turn/completed`.
  - Item lifecycle: `item/started`, item-specific deltas, `item/completed`.
- `thread/realtime/*` is transport-ephemeral and is not persisted as `ThreadItem`.
- Token usage streams separately via `thread/tokenUsage/updated`.

### Notification Opt-Out

`initialize.params.capabilities.optOutNotificationMethods` supports per-connection suppression.

- Exact method-name matching only (no prefixes/wildcards).
- Unknown method names are accepted/ignored.
- Applies to notifications only; request/response/error frames are unaffected.

### Experimental Event Families

- Fuzzy file search:
  - `fuzzyFileSearch/sessionUpdated` with `{ sessionId, query, files }`.
  - `fuzzyFileSearch/sessionCompleted` with `{ sessionId, query }`.
- Thread realtime:
  - `thread/realtime/started`
  - `thread/realtime/itemAdded`
  - `thread/realtime/transcriptUpdated`
  - `thread/realtime/outputAudio/delta`
  - `thread/realtime/error`
  - `thread/realtime/closed`
- Windows sandbox setup:
  - `windowsSandbox/setupCompleted` with `{ mode, success, error }`.
- MCP startup:
  - `mcpServer/startupStatus/updated` with `{ name, status, error }`, where `status` is `starting | ready | failed | cancelled`.

### Turn-Level Detail

- `turn/diff/updated`:
  - Streams latest aggregated unified diff snapshot for the turn.
- `turn/plan/updated`:
  - Streams structured plan updates (`pending | inProgress | completed`).
- `model/rerouted`:
  - Notifies backend model reroute (`fromModel`, `toModel`, `reason`).
- Current caveat:
  - `turn/started` and `turn/completed` payloads may still show empty `items`; treat `item/*` notifications as canonical for streamed item state.

### Item Types And Deltas

Common `ThreadItem` types include:

- `userMessage`
- `agentMessage`
- `plan`
- `reasoning`
- `commandExecution`
- `fileChange`
- `mcpToolCall`
- `collabToolCall`
- `webSearch`
- `imageView`
- `enteredReviewMode`
- `exitedReviewMode`
- `contextCompaction`
- `compacted` (deprecated in favor of `contextCompaction`)

Item-specific deltas include:

- `item/agentMessage/delta`
- `item/plan/delta` (experimental)
- `item/reasoning/summaryTextDelta`
- `item/reasoning/summaryPartAdded`
- `item/reasoning/textDelta`
- `item/commandExecution/outputDelta`
- `item/fileChange/outputDelta`

Guardian auto-approval review notifications are currently unstable:

- `item/autoApprovalReview/started`
- `item/autoApprovalReview/completed`

### Errors

- Mid-turn errors are emitted via `error` with same shape used by failed `turn/completed`.
- `codexErrorInfo` variants include:
  - `ContextWindowExceeded`
  - `UsageLimitExceeded`
  - `HttpConnectionFailed`
  - `ResponseStreamConnectionFailed`
  - `ResponseStreamDisconnected`
  - `ResponseTooManyFailedAttempts`
  - `ActiveTurnNotSteerable`
  - `BadRequest`
  - `Unauthorized`
  - `SandboxError`
  - `InternalServerError`
  - `Other`
- Upstream status may appear as `httpStatusCode` on relevant variants.

## Approvals

Certain actions require explicit client/user decisions. The server issues JSON-RPC requests; the client responds once with a decision payload.

Shared approval behavior:

- Requests include `threadId` and `turnId` to scope UI state.
- After resolution or lifecycle cleanup, server emits `serverRequest/resolved { threadId, requestId }`.
- Final execution state is authoritative on corresponding `item/completed`.

### Command Execution Approval Flow

1. `item/started` for pending `commandExecution`.
2. `item/commandExecution/requestApproval` (request):
   - Includes identifiers, reason, command context, and optional policy amendment hints.
   - With experimental capability, may include `additionalPermissions`.
3. Client decision:
   - `accept`
   - `acceptForSession`
   - `acceptWithExecpolicyAmendment`
   - `applyNetworkPolicyAmendment`
   - `decline`
   - `cancel`
4. `serverRequest/resolved`.
5. Final `item/completed` with command status/output.

### File Change Approval Flow

1. `item/started` with proposed `fileChange`.
2. `item/fileChange/requestApproval` request.
3. Client decision:
   - `accept`
   - `acceptForSession`
   - `decline`
   - `cancel`
4. `serverRequest/resolved`.
5. Final `item/completed` with `completed | failed | declined`.

### request_user_input Tool Handshake

- For `item/tool/requestUserInput`, the request is resolved by user response.
- If cleared by turn start/completion/interruption first, server still emits `serverRequest/resolved`.

### MCP Elicitation Flow

- Server request: `mcpServer/elicitation/request` with either:
  - `mode: "form"` + `requestedSchema`, or
  - `mode: "url"` + URL payload.
- Client response action:
  - `accept` (with content)
  - `decline`
  - `cancel`
- Server emits `serverRequest/resolved` after answer or cleanup.
- `turnId` can be `null` when not correlated to an active turn.

For MCP tool approval elicitations, metadata can include `codex_approval_kind: "mcp_tool_call"` and optional persistence hints (`session`, `always`).

### Permission Requests (`request_permissions` Tool)

- Server request method: `item/permissions/requestApproval`.
- Request carries requested permission profile (filesystem and/or network) plus reason/context.
- Client returns granted subset in `result.permissions`.
- Optional `result.scope`:
  - `"turn"` (default behavior)
  - `"session"` (persist granted permissions for later turns)

### Dynamic Tool Calls (Experimental)

- Experimental tool-calling paths can produce approval-like interactions and resolution cleanup through the same `serverRequest/resolved` lifecycle.

