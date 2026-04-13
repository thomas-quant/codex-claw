# Codex App-Server Harness Spec

This document is a clean-room specification of how the `@openclaw/codex` package works, based on the package source and tests in this repository.

It describes the externally meaningful behavior of the module rather than restating implementation details line by line.

## Purpose

The package bridges OpenClaw to a Codex app-server runtime.

It provides three plugin surfaces:

- An agent harness that runs Codex app-server turns.
- A provider that exposes Codex model metadata to OpenClaw.
- A `codex` command for inspection and control flows.

Primary entrypoints:

- [index.ts](./index.ts)
- [harness.ts](./harness.ts)
- [provider.ts](./provider.ts)
- [src/commands.ts](./src/commands.ts)

## Plugin Surface

The plugin registers:

- `registerAgentHarness(createCodexAppServerAgentHarness(...))`
- `registerProvider(buildCodexProvider(...))`
- `registerCommand(createCodexCommand(...))`

The default harness and provider id is `codex`.

The harness only claims support for configured provider ids and rejects other providers.

## Configuration

Configuration is declared in [openclaw.plugin.json](./openclaw.plugin.json) and normalized in [src/app-server/config.ts](./src/app-server/config.ts).

Supported top-level sections:

- `discovery`
- `appServer`

`discovery` controls live model discovery:

- `enabled`
- `timeoutMs`

`appServer` controls how OpenClaw connects to Codex app-server:

- `transport`: `stdio` or `websocket`
- `command`
- `args`
- `url`
- `authToken`
- `headers`
- `requestTimeoutMs`
- `approvalPolicy`
- `sandbox`
- `approvalsReviewer`
- `serviceTier`

Default runtime behavior:

- Transport defaults to `stdio`.
- Command defaults to `codex`.
- Args default to `["app-server", "--listen", "stdio://"]`.
- Request timeout defaults to `60000`.
- Approval policy defaults to `never`.
- Sandbox defaults to `workspace-write`.
- Approvals reviewer defaults to `user`.

Environment variables can override part of the runtime config, but typed plugin config wins when both are present.

If `transport` is `websocket`, `url` is required.

## Runtime Architecture

At a high level the package is composed of five layers:

1. Plugin entry and provider registration.
2. Shared app-server client and transport layer.
3. Session-to-thread binding and lifecycle management.
4. Per-attempt execution pipeline.
5. Result projection and transcript mirroring.

Important modules:

- [src/app-server/client.ts](./src/app-server/client.ts)
- [src/app-server/shared-client.ts](./src/app-server/shared-client.ts)
- [src/app-server/thread-lifecycle.ts](./src/app-server/thread-lifecycle.ts)
- [src/app-server/run-attempt.ts](./src/app-server/run-attempt.ts)
- [src/app-server/event-projector.ts](./src/app-server/event-projector.ts)

## Provider and Model Catalog

The provider exposes Codex models as `openai-codex-responses` models.

Catalog behavior:

- Prefer live discovery through the app-server.
- If discovery is disabled or fails, fall back to a bundled static list.
- Filter out hidden models from discovered results.
- Treat discovery as best-effort rather than mandatory.

Bundled fallback models currently include:

- `gpt-5.4`
- `gpt-5.4-mini`
- `gpt-5.2`

Dynamic model resolution accepts arbitrary non-empty Codex model ids and synthesizes OpenClaw model metadata for them.

Discovery entrypoints:

- [provider.ts](./provider.ts)
- [src/app-server/models.ts](./src/app-server/models.ts)

## Transport and Client Protocol

The transport layer supports two connection modes:

- `stdio`, which spawns the Codex binary as a child process.
- `websocket`, which connects to an already-running app-server.

The client expects a minimal transport abstraction with:

- writable `stdin`
- readable `stdout`
- readable `stderr`
- process lifecycle hooks such as `once`, `kill`, and `unref`

The wire protocol is newline-delimited JSON-RPC-like messaging:

- Requests: `{ id?, method, params? }`
- Responses: `{ id, result? , error? }`
- Notifications: `{ method, params? }`

Client requirements:

- The connection must be initialized before normal RPC traffic is relied on.
- The app-server version must be at least `0.118.0`.
- Requests support timeout and abort semantics.
- Late responses after timeout or abort are ignored.
- Server-initiated requests can be handled by registered request handlers.
- Unhandled approval-style requests fail closed by default.

Relevant modules:

- [src/app-server/transport.ts](./src/app-server/transport.ts)
- [src/app-server/transport-stdio.ts](./src/app-server/transport-stdio.ts)
- [src/app-server/transport-websocket.ts](./src/app-server/transport-websocket.ts)
- [src/app-server/client.ts](./src/app-server/client.ts)

## Shared Client Semantics

The package maintains a process-global shared app-server client.

Shared client behavior:

- Cache one initialized client per effective start-option shape.
- Reuse the client across compatible callers.
- Clear the client on startup failure, handshake failure, timeout, close, or explicit disposal.
- Clear and recreate the client when the effective start options change.

Live model discovery explicitly tears down the shared client after each discovery pass so catalog discovery stays transient.

Primary module:

- [src/app-server/shared-client.ts](./src/app-server/shared-client.ts)

## Session and Thread State

The package tracks state in two places:

- The OpenClaw session transcript file.
- A sidecar Codex thread-binding file.

The sidecar binding path is:

- `<sessionFile>.codex-app-server.json`

Binding data includes:

- `threadId`
- `cwd`
- optional `model`
- optional `modelProvider`
- optional `dynamicToolsFingerprint`
- `createdAt`
- `updatedAt`

The binding file is the local cache of which Codex thread belongs to which OpenClaw session file.

Primary module:

- [src/app-server/session-binding.ts](./src/app-server/session-binding.ts)

## Thread Lifecycle

Before running a turn, the harness either resumes an existing Codex thread or starts a fresh one.

Resume/start rules:

- Read the sidecar binding for the current session file.
- Compute a fingerprint for the current dynamic tool catalog.
- If a binding exists and its fingerprint is absent or matches, attempt `thread/resume`.
- If resume fails, clear the binding and fall back to `thread/start`.
- If the stored fingerprint exists but differs from the current one, invalidate the binding and start a new thread.

Both resume and fresh start carry forward the current OpenClaw model selection and app-server policy fields.

Both resume and fresh start preserve extended history.

Primary module:

- [src/app-server/thread-lifecycle.ts](./src/app-server/thread-lifecycle.ts)

## Attempt Execution Pipeline

The harness execution path is [src/app-server/run-attempt.ts](./src/app-server/run-attempt.ts).

Per-attempt flow:

1. Resolve workspace and sandbox context.
2. Resolve upstream abort handling.
3. Build the dynamic tool catalog.
4. Acquire or start the app-server client.
5. Start or resume the Codex thread.
6. Register notification and request handlers.
7. Call `turn/start`.
8. Create an active embedded run handle.
9. Wait for the matching `turn/completed`.
10. Build the OpenClaw result from projected events and tool telemetry.
11. Mirror transcript state back into the local session file on a best-effort basis.

The run handle supports:

- `queueMessage`, which becomes `turn/steer`
- `abort`
- `cancel`
- `isStreaming`
- `isCompacting`

Aborting a run sends `turn/interrupt` for the active thread and turn.

## Event Projection

The app-server is treated as the canonical source of turn state.

Notifications are projected into OpenClaw result state by [src/app-server/event-projector.ts](./src/app-server/event-projector.ts).

Projection rules:

- Ignore notifications for other threads or turns.
- Accumulate assistant deltas by item id.
- Expose only the last non-empty assistant item as the final visible assistant reply.
- Stream reasoning deltas through reasoning callbacks.
- Convert plan deltas and plan updates into OpenClaw plan events.
- Convert item lifecycle notifications into OpenClaw item events.
- Track compaction start and end via dedicated compaction events.
- Track token usage updates and attach normalized usage to the final attempt result.
- Track guardian auto-approval review events separately.

The resulting local `messagesSnapshot` always starts with the user prompt and may include mirrored assistant entries for reasoning and plan history.

## Dynamic Tools

Dynamic tools are only exposed when:

- tools are not disabled
- the selected model supports tools

The effective tool set may be reduced further by an allowlist.

Tool behavior:

- The selected tools are sent to the app-server as `dynamicTools` during thread start.
- Incoming `item/tool/call` requests are matched to OpenClaw tools by name.
- Unknown tools fail closed.
- Tool execution errors fail closed.
- Successful tool results are converted into app-server content items.
- Tool telemetry is recorded locally for messaging side effects, media artifacts, voice flags, and cron adds.

Primary module:

- [src/app-server/dynamic-tools.ts](./src/app-server/dynamic-tools.ts)

## Approval Flow

Native app-server approval requests are intercepted and bridged into OpenClaw approval machinery.

Approval behavior:

- Only handle approvals for the active thread and turn.
- Build a human-readable approval context from command, reason, item id, or call id.
- Route the request through OpenClaw plugin approval tools.
- Wait for a decision when needed.
- Map the resulting approval outcome into the exact response schema expected by the original app-server request family.
- Fail closed when the route is unavailable, cancelled, or denied.

Approval request families handled explicitly include:

- command execution approval
- file change approval
- permissions approval

Primary module:

- [src/app-server/approval-bridge.ts](./src/app-server/approval-bridge.ts)

## Transcript Mirroring

The Codex thread is treated as the canonical remote conversation, but local OpenClaw history is still maintained.

Transcript mirroring behavior:

- Mirror only `user` and `assistant` messages.
- Append them into the local session file.
- Use idempotency keys so the same turn is not mirrored twice.
- Emit a session transcript update after a successful append.
- Swallow mirroring failures after logging so they do not fail the attempt.

Primary module:

- [src/app-server/transcript-mirror.ts](./src/app-server/transcript-mirror.ts)

## Compaction, Reset, and Dispose

Compaction uses native app-server thread compaction rather than local summarization.

Compaction behavior:

- Read the bound thread id from the sidecar binding.
- Send `thread/compact/start`.
- Wait for either `thread/compacted` or completion of a `contextCompaction` item for the same thread.
- Return success only after one of those completion signals is observed.

Reset behavior:

- Delete the sidecar binding for the session file.

Dispose behavior:

- Tear down the shared app-server client.

Primary modules:

- [src/app-server/compact.ts](./src/app-server/compact.ts)
- [harness.ts](./harness.ts)

## Test-Backed Invariants

The test suite establishes the following important guarantees:

- The client routes responses by request id and preserves RPC error codes.
- Unsupported or missing app-server versions are rejected during initialize.
- Websocket transport supports authenticated startup and request flow.
- Startup timeout and turn-start timeout fail closed.
- Turn completion notifications that arrive early are buffered rather than lost.
- Aborts become `turn/interrupt`.
- Resume paths preserve extended history and current policy/model fields.
- Final assistant output hides intermediate commentary and uses the terminal assistant item.
- Approval bridging fails closed when approval routing is unavailable.
- Dynamic tool telemetry only records successful side effects.
- Transcript mirroring is idempotent by turn-specific scope.
- Native compaction waits for an explicit completion signal.

Representative tests:

- [src/app-server/client.test.ts](./src/app-server/client.test.ts)
- [src/app-server/run-attempt.test.ts](./src/app-server/run-attempt.test.ts)
- [src/app-server/event-projector.test.ts](./src/app-server/event-projector.test.ts)
- [src/app-server/approval-bridge.test.ts](./src/app-server/approval-bridge.test.ts)
- [src/app-server/dynamic-tools.test.ts](./src/app-server/dynamic-tools.test.ts)
- [src/app-server/transcript-mirror.test.ts](./src/app-server/transcript-mirror.test.ts)
- [src/app-server/compact.test.ts](./src/app-server/compact.test.ts)

## Known Ambiguities

The current implementation leaves a few edges that are not fully pinned down by tests:

- Shared-client caching keys auth token presence, not token value.
- Live model discovery does not follow pagination via `nextCursor`.
- Dynamic-tool fingerprint invalidation exists, but that branch is lightly tested.
- The final-assistant-item heuristic is order-sensitive and only lightly tested for unusual ordering cases.
- Transcript mirroring is explicitly advisory and not part of success semantics.

## Repository Caveat

The source module appears internally coherent, but this checkout currently lacks the base TypeScript config referenced by [tsconfig.json](./tsconfig.json), so a full compile could not be verified from this package alone.
