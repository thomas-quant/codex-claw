## API Overview

This is a compressed technical reference for the app-server API surface.
It removes long examples while preserving method-level behavior and constraints.

### Thread Lifecycle

- `thread/start`:
  - Creates a new thread, emits `thread/started`, and auto-subscribes this connection to thread events.
  - Supports runtime overrides (`model`, `cwd`, sandbox/approval settings, etc).
  - If `cwd` is provided and sandbox resolves to `workspace-write` or full access, the project is marked trusted in user config.
  - `sessionStartSource: "clear"` is available for clear-session starts.
- `thread/resume`:
  - Reopens an existing thread id so future `turn/start` calls append to that thread.
- `thread/fork`:
  - Copies a thread into a new thread id (can preserve interruption semantics if source is mid-turn).
  - Supports `ephemeral: true` for in-memory temporary forks.
  - Emits `thread/started` for the forked thread.
- `thread/list`:
  - Cursor pagination with filters: `modelProviders`, `sourceKinds`, `archived`, `cwd`, `searchTerm`.
  - Returned `thread.status` is `ThreadStatus`; unloaded threads default to `notLoaded`.
- `thread/loaded/list`:
  - Returns thread ids currently loaded in memory.
- `thread/read`:
  - Reads stored thread without resuming; optional `includeTurns`.
  - Returns `ThreadStatus` like `thread/list`.
- `thread/metadata/update`:
  - Patches persisted metadata (currently `gitInfo`) in sqlite and returns refreshed thread.
- `thread/status/changed`:
  - Notification for loaded-thread status transitions.
- `thread/archive`:
  - Moves rollout into archived storage and emits `thread/archived`.
- `thread/unarchive`:
  - Restores archived rollout, returns restored `thread`, emits `thread/unarchived`.
- `thread/name/set`:
  - Sets user-facing thread name for loaded or persisted thread and emits `thread/name/updated`.
- `thread/unsubscribe`:
  - Unsubscribes this connection from thread events.
  - If last subscriber leaves, thread is unloaded and `thread/closed` is emitted.
- `thread/compact/start`:
  - Starts async context compaction and returns `{}` immediately.
  - Progress is streamed through normal `turn/*` and `item/*` events.
- `thread/rollback`:
  - Drops last N turns from in-memory context and persists a rollback marker.
  - Returns updated thread (with turns populated).

### Turn Control And Realtime

- `turn/start`:
  - Starts a new turn on a thread with mixed input items (`text`, `image`, `localImage`, `skill`, `mention`).
  - Emits `turn/started`, item stream, then `turn/completed`.
  - Supports per-turn overrides and optional schema-constrained output.
- `turn/steer`:
  - Adds user input to an already in-flight steerable turn.
  - Rejects non-steerable active turn kinds (for example review/manual compaction).
- `turn/interrupt`:
  - Cancels an in-flight turn by `(threadId, turnId)`.
  - Turn ends with `status: "interrupted"`.
- `thread/realtime/start` (experimental):
  - Starts thread-scoped realtime session over websocket default transport or WebRTC offer/answer (`transport: webrtc`).
  - Remote answer SDP is emitted via `thread/realtime/sdp`.
- `thread/realtime/appendAudio` (experimental):
  - Appends input audio chunk to active realtime session.
- `thread/realtime/appendText` (experimental):
  - Appends text input to active realtime session.
- `thread/realtime/stop` (experimental):
  - Stops active realtime session.
- `thread/backgroundTerminals/clean` (experimental, requires `experimentalApi`):
  - Terminates all running background terminals for a thread.

### Review, Shell, And One-Off Command Execution

- `review/start`:
  - Starts automated reviewer flow; streams review-mode items and final `agentMessage`.
- `thread/shellCommand`:
  - Runs user `!` command against a thread.
  - Explicitly unsandboxed (full access), not inherited from thread sandbox policy.
  - Streams as normal thread item events and can inject formatted output into active turn stream.
- `command/exec`:
  - Runs one sandboxed command without creating a thread/turn.
  - Useful for utility execution/validation.
- `command/exec/write`:
  - Writes base64-decoded stdin bytes (or closes stdin) to running `command/exec`.
- `command/exec/resize`:
  - Resizes a PTY-backed `command/exec` process by `processId`.
- `command/exec/terminate`:
  - Terminates running `command/exec` by `processId`.
- `command/exec/outputDelta`:
  - Notification with base64 stdout/stderr chunks for streaming command output.

### Filesystem API

All `fs/*` methods use absolute paths.

- `fs/readFile`: returns base64 file bytes (`dataBase64`).
- `fs/writeFile`: writes base64 file bytes.
- `fs/createDirectory`: creates directories (`recursive` defaults to `true`).
- `fs/getMetadata`: returns `isDirectory`, `isFile`, `createdAtMs`, `modifiedAtMs`.
- `fs/readDirectory`: lists direct children (`fileName`, `isDirectory`, `isFile`).
- `fs/remove`: removes file/directory trees (`recursive` and `force` default to `true`).
- `fs/copy`: copies files/directories (directory copy requires `recursive: true`).
- `fs/watch`: subscribes to change events for a path and caller-provided `watchId`; returns canonicalized `path`.
- `fs/unwatch`: unsubscribes a prior watch.
- `fs/changed`: notification with `{ watchId, changedPaths }`.

### Models, Features, Modes, Skills, Plugins, Apps

- `model/list`:
  - Lists models, optional hidden entries, reasoning-effort options, speed tiers, and optional upgrade metadata.
- `experimentalFeature/list`:
  - Lists feature flags with stage metadata and pagination.
- `experimentalFeature/enablement/set`:
  - Patches in-memory process-wide feature enablement for supported keys (currently `apps`, `plugins`).
- `collaborationMode/list` (experimental):
  - Lists collaboration mode presets; built-in developer instructions are intentionally omitted from this response.
- `skills/list`:
  - Lists skills for one or more `cwd` values, optional `forceReload`.
- `skills/changed`:
  - Notification when watched local skill files change.
- `skills/config/write`:
  - Writes user-level skill config by name or absolute path.
- `plugin/list` (under development):
  - Lists discovered marketplaces and plugin state, including policy and load errors.
  - `forceRemoteSync: true` refreshes curated state before listing.
- `plugin/read` (under development):
  - Reads plugin by `marketplacePath` and `pluginName`, including summary/interface/skills/apps metadata.
- `plugin/install` (under development):
  - Installs plugin from discovered marketplace entry and installs bundled MCPs if present.
- `plugin/uninstall` (under development):
  - Uninstalls plugin and clears user-level config entry.
- `app/list`:
  - Lists available apps/connectors.

### MCP, Config, Sandbox Setup, Feedback, Migration

- `mcpServer/oauth/login`:
  - Starts OAuth login for a configured MCP server; completion arrives via notification.
- `tool/requestUserInput` (experimental):
  - Prompts user with 1-3 short questions and returns answers.
- `config/mcpServer/reload`:
  - Reloads MCP config from disk and queues refresh for loaded threads (applied on next active turn).
- `mcpServerStatus/list`:
  - Enumerates configured MCP servers, tools, auth status, and optional resources/templates.
- `mcpServer/resource/read`:
  - Reads MCP resource by `threadId`, `server`, and `uri`.
- `mcpServer/tool/call`:
  - Calls MCP tool by `threadId`, `server`, `tool`, optional `arguments` and `_meta`.
- `windowsSandbox/setupStart`:
  - Starts Windows sandbox setup (`elevated` or `unelevated`), optional absolute `cwd`.
  - Returns `{ started: true }`; completion emitted asynchronously.
- `feedback/upload`:
  - Uploads feedback report with optional logs and attachments; returns tracking thread id.
- `config/read`:
  - Returns effective on-disk config after layering resolution.
- `config/value/write`:
  - Writes one key/value in user `config.toml`.
- `config/batchWrite`:
  - Applies multiple config edits atomically, optional hot reload.
- `configRequirements/read`:
  - Reads requirements constraints from `requirements.toml` and/or MDM (if configured).
- `externalAgentConfig/detect`:
  - Detects migratable external-agent artifacts (`includeHome`, optional `cwds`).
- `externalAgentConfig/import`:
  - Imports selected migration items.

### Minimal Request Skeletons

```json
{ "method": "thread/start", "id": 10, "params": { "cwd": "/abs/project", "model": "gpt-5.1-codex" } }
```

```json
{ "method": "turn/start", "id": 20, "params": { "threadId": "thr_123", "input": [{ "type": "text", "text": "hello" }] } }
```

```json
{ "method": "command/exec", "id": 30, "params": { "command": ["ls", "-la"], "cwd": "/abs/project" } }
```

