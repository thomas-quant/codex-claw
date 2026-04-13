# codex-app-server

`codex app-server` is the interface Codex uses to power rich interfaces such as the [Codex VS Code extension](https://marketplace.visualstudio.com/items?itemName=openai.chatgpt).

## Table of Contents

- [Protocol](#protocol)
- [Message Schema](#message-schema)
- [Core Primitives](#core-primitives)
- [Lifecycle Overview](#lifecycle-overview)

For the remaining sections, see the index: [official-summary.md](./official-summary.md).

## Protocol

Similar to [MCP](https://modelcontextprotocol.io/), `codex app-server` supports bidirectional communication using JSON-RPC 2.0 messages (with the `"jsonrpc":"2.0"` header omitted on the wire).

Supported transports:

- stdio (`--listen stdio://`, default): newline-delimited JSON (JSONL)
- websocket (`--listen ws://IP:PORT`): one JSON-RPC message per websocket text frame (**experimental / unsupported**)
- off (`--listen off`): do not expose a local transport

When running with `--listen ws://IP:PORT`, the same listener also serves basic HTTP health probes:

- `GET /readyz` returns `200 OK` once the listener is accepting new connections.
- `GET /healthz` returns `200 OK` when no `Origin` header is present.
- Any request carrying an `Origin` header is rejected with `403 Forbidden`.

Websocket transport is currently experimental and unsupported. Do not rely on it for production workloads.

Security note:

- Loopback websocket listeners (`ws://127.0.0.1:PORT`) remain appropriate for localhost and SSH port-forwarding workflows.
- Non-loopback websocket listeners currently allow unauthenticated connections by default during rollout. If you expose one remotely, configure websocket auth explicitly now.
- Supported auth modes are app-server flags:
  - `--ws-auth capability-token --ws-token-file /absolute/path`
  - `--ws-auth signed-bearer-token --ws-shared-secret-file /absolute/path` for HMAC-signed JWT/JWS bearer tokens, with optional `--ws-issuer`, `--ws-audience`, `--ws-max-clock-skew-seconds`
- Clients present the credential as `Authorization: Bearer <token>` during the websocket handshake. Auth is enforced before JSON-RPC `initialize`.

Tracing/log output:

- `RUST_LOG` controls log filtering/verbosity.
- Set `LOG_FORMAT=json` to emit app-server tracing logs to `stderr` as JSON (one event per line).

Backpressure behavior:

- The server uses bounded queues between transport ingress, request processing, and outbound writes.
- When request ingress is saturated, new requests are rejected with a JSON-RPC error code `-32001` and message `"Server overloaded; retry later."`.
- Clients should treat this as retryable and use exponential backoff with jitter.

## Message Schema

Currently, you can dump a TypeScript version of the schema using `codex app-server generate-ts`, or a JSON Schema bundle via `codex app-server generate-json-schema`. Each output is specific to the version of Codex you used to run the command, so the generated artifacts are guaranteed to match that version.

```
codex app-server generate-ts --out DIR
codex app-server generate-json-schema --out DIR
```

## Core Primitives

The API exposes three top level primitives representing an interaction between a user and Codex:

- **Thread**: A conversation between a user and the Codex agent. Each thread contains multiple turns.
- **Turn**: One turn of the conversation, typically starting with a user message and finishing with an agent message. Each turn contains multiple items.
- **Item**: Represents user inputs and agent outputs as part of the turn, persisted and used as the context for future conversations. Example items include user message, agent reasoning, agent message, shell command, file edit, etc.

Use the thread APIs to create, list, or archive conversations. Drive a conversation with turn APIs and stream progress via turn notifications.

## Lifecycle Overview

- Initialize once per connection: Immediately after opening a transport connection, send an `initialize` request with your client metadata, then emit an `initialized` notification. Any other request on that connection before this handshake gets rejected.
- Start (or resume) a thread: Call `thread/start` to open a fresh conversation. The response returns the thread object and you’ll also get a `thread/started` notification. If you’re continuing an existing conversation, call `thread/resume` with its ID instead. If you want to branch from an existing conversation, call `thread/fork` to create a new thread id with copied history. Like `thread/start`, `thread/fork` also accepts `ephemeral: true` for an in-memory temporary thread.
  The returned `thread.ephemeral` flag tells you whether the session is intentionally in-memory only; when it is `true`, `thread.path` is `null`.
- Begin a turn: To send user input, call `turn/start` with the target `threadId` and the user's input. Optional fields let you override model, cwd, sandbox policy, approval policy, approvals reviewer, etc. This immediately returns the new turn object. The app-server emits `turn/started` when that turn actually begins running.
- Stream events: After `turn/start`, keep reading JSON-RPC notifications on stdout. You’ll see `item/started`, `item/completed`, deltas like `item/agentMessage/delta`, tool progress, etc. These represent streaming model output plus any side effects (commands, tool calls, reasoning notes).
- Finish the turn: When the model is done (or the turn is interrupted via making the `turn/interrupt` call), the server sends `turn/completed` with the final turn state and token usage.
