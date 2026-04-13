# Codex Tool And Approval Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Teach the Codex app-server runtime to execute PicoClaw tools during a live turn, auto-answer native approval requests with permanent-YOLO semantics, and keep the existing agent-loop hook/tool/session behavior.

**Architecture:** Keep Codex behind the provider abstraction. Add a small interactive tool-callback contract to `providers.InteractiveTurnRequest`, extend `pkg/codexruntime` so `RunTextTurn` can answer server-initiated JSON-RPC requests, and reuse the current agent-loop tool execution path by extracting it behind a helper instead of inventing a second tool system.

**Tech Stack:** Go 1.25, Codex app-server JSON-RPC over `stdio`, existing `pkg/tools`, `pkg/agent/hooks`, `pkg/isolation`, `go test`

---

## Scope Split

Included in this plan:

- Codex dynamic tool advertisement on `thread/start`
- JSON-RPC handling for `item/tool/call`
- native approval auto-responses for command/file-change/permissions approval requests
- agent-loop callback integration that reuses PicoClaw tool hooks, approval hooks, result shaping, and session logging
- focused unit tests plus one narrow integration test

Excluded from this plan:

- slash commands and persistent per-thread runtime settings
- compaction policy changes beyond preserving the current foundation
- config rewrite and deletion passes
- MCP allowlist enforcement

## File Structure

### Create

- `pkg/codexruntime/tool_bridge.go` — runtime-side tool request/result types plus dynamic tool definition mapping
- `pkg/codexruntime/tool_bridge_test.go` — focused tool-call response shaping tests
- `pkg/codexruntime/approval_bridge.go` — native approval request parsing and permanent-YOLO replies
- `pkg/codexruntime/approval_bridge_test.go` — focused approval-family reply tests
- `pkg/agent/interactive_tool_exec.go` — extracted helper that executes one interactive tool call using the current loop machinery

### Modify

- `pkg/codexruntime/protocol.go`
- `pkg/codexruntime/client.go`
- `pkg/codexruntime/client_test.go`
- `pkg/codexruntime/runner.go`
- `pkg/codexruntime/runner_test.go`
- `pkg/providers/types.go`
- `pkg/providers/codex_app_server_provider.go`
- `pkg/providers/codex_app_server_provider_test.go`
- `pkg/agent/loop.go`
- `pkg/agent/loop_test.go`

## Contract To Preserve

- Tool lookup is exact-name only.
- Unknown tool name returns a normal tool failure payload, not a JSON-RPC error.
- Tool execution errors return `success=false` plus one text content item.
- Native approval replies stay protocol-correct:
  - command/file change: `{decision:"accept"}`
  - permissions: `{permissions:<requested>, scope:"turn"}`
- Reasoning remains hidden from channel streaming.
- PicoClaw session logging and hook behavior stay authoritative.
- Do **not** invalidate a saved thread only because the tool catalog changed; keep the same thread and refresh the advertised catalog on the next active turn.

## Task 1: Add Interactive Tool Callback Plumbing

**Files:**
- Modify: `pkg/providers/types.go`
- Modify: `pkg/providers/codex_app_server_provider.go`
- Modify: `pkg/providers/codex_app_server_provider_test.go`
- Modify: `pkg/codexruntime/runner.go`
- Modify: `pkg/codexruntime/runner_test.go`

- [ ] Add minimal provider-layer types for one interactive tool call and its response.
- [ ] Extend `InteractiveTurnRequest` with a tool execution callback and keep `OnChunk` behavior unchanged.
- [ ] Extend `codexruntime.RunRequest` with:
  - advertised dynamic tools
  - optional tool call handler
  - native approval policy/handler inputs needed by the runtime
- [ ] Update `CodexAppServerProvider.RunInteractiveTurn` to:
  - map `[]providers.ToolDefinition` to runtime dynamic tools
  - forward the interactive tool callback into the runner
  - keep binding-key and last-user-message behavior unchanged
- [ ] Add focused provider tests for:
  - tool definition mapping
  - forwarded tool callback invocation

Run:

```bash
/tmp/go/bin/go test ./pkg/providers -run 'TestCodexAppServerProvider' -count=1
```

## Task 2: Teach codexruntime To Answer Server Requests

**Files:**
- Create: `pkg/codexruntime/tool_bridge.go`
- Create: `pkg/codexruntime/tool_bridge_test.go`
- Create: `pkg/codexruntime/approval_bridge.go`
- Create: `pkg/codexruntime/approval_bridge_test.go`
- Modify: `pkg/codexruntime/protocol.go`
- Modify: `pkg/codexruntime/client.go`
- Modify: `pkg/codexruntime/client_test.go`

- [ ] Add protocol types for:
  - dynamic tool definitions advertised at thread start
  - `item/tool/call` request params/result
  - command/file-change/permissions approval request params
  - text/image content-item replies
- [ ] Replace the current “reject any server request during a turn” path in `Client.RunTextTurn` with request routing:
  - tool calls go to the tool bridge
  - approval requests go to the approval bridge
  - unknown server requests still get a clean failure reply, not a dead connection
- [ ] Keep the current-thread / current-turn guard when handling tool and approval requests.
- [ ] Make `thread/start` advertise `dynamicTools` and permanent-YOLO approval policy.
- [ ] Add focused client/runtime tests for:
  - one successful tool call response
  - unknown tool response
  - one permissions approval response
  - one unsupported server request fallback

Run:

```bash
/tmp/go/bin/go test ./pkg/codexruntime -run 'TestClient_RunTextTurn|TestToolBridge|TestApprovalBridge' -count=1
```

## Task 3: Reuse The Existing Agent Tool Execution Path

**Files:**
- Create: `pkg/agent/interactive_tool_exec.go`
- Modify: `pkg/agent/loop.go`
- Modify: `pkg/agent/loop_test.go`

- [ ] Extract the current single-tool execution block from `runTurn` into a helper that:
  - accepts one tool name, arguments, and call id
  - runs `BeforeTool`, `ApproveTool`, actual tool execution, `AfterTool`, session logging, media normalization, and event emission
  - returns the shaped Codex-facing content items plus success flag
- [ ] Keep the helper in the agent layer; do not import `pkg/tools` into provider types.
- [ ] When building `providers.InteractiveTurnRequest`, attach a callback that calls the new helper.
- [ ] Preserve current loop behavior for normal non-interactive providers.
- [ ] Add one narrow integration test where an interactive provider invokes the callback once and the loop logs/returns the expected tool result without using the old stateless `Chat` path.

Run:

```bash
/tmp/go/bin/go test ./pkg/agent -run 'TestAgentLoop_UsesInteractiveProvider' -count=1
```

## Task 4: Focused End-To-End Verification

**Files:**
- Modify as needed from Tasks 1-3

- [ ] Run only the touched packages first.
- [ ] If those pass, run the combined targeted set below.
- [ ] Do not widen to full-repo verification in this phase unless a touched-package test reveals collateral damage.

Run:

```bash
/tmp/go/bin/go test ./pkg/codexruntime ./pkg/providers ./pkg/agent -count=1
```

Expected:

- `pkg/codexruntime` passes with server-request coverage
- `pkg/providers` passes with callback forwarding coverage
- `pkg/agent` passes with the interactive tool-callback integration test

## Worker Split

Use disjoint write sets:

1. Runtime/provider worker
   - owns `pkg/codexruntime/*`
   - owns `pkg/providers/types.go`
   - owns `pkg/providers/codex_app_server_provider.go`
   - owns `pkg/providers/codex_app_server_provider_test.go`

2. Agent-loop worker
   - owns `pkg/agent/interactive_tool_exec.go`
   - owns `pkg/agent/loop.go`
   - owns `pkg/agent/loop_test.go`

Shared contract between workers:

- provider callback shape lives in `pkg/providers/types.go`
- runtime calls the callback synchronously for each `item/tool/call`
- callback returns Codex-facing content items plus `success`
- approval handling stays inside `pkg/codexruntime`, not the agent loop
