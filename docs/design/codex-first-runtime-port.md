# Codex-First Runtime Port Design

Status: Draft

Related documents:

- [Codex App-Server Harness Spec](/root/picoclaw/reference/codex-app-server/harness-spec.md)
- [codex-app-serv.md](/root/picoclaw/reference/codex-app-server/official-summary.md)
- [codex-app-serv.part-01.md](/root/picoclaw/reference/codex-app-server/official-summary.part-01.md)
- [codex-app-serv.part-05.md](/root/picoclaw/reference/codex-app-server/official-summary.part-05.md)

## 1. Purpose

This document defines the PicoClaw fork as a Codex-first runtime.

The fork keeps PicoClaw's agent, workspace, tool, memory, cron, MCP, and channel backbone, but replaces the current
general-purpose provider stack with a narrow runtime:

- primary model runtime: Codex app-server over `stdio`
- fallback model runtime: DeepSeek over HTTP
- supported channels: Telegram and Discord
- no web launcher or browser auth surface

This is a constrained behavioral port. OpenClaw remains the semantic reference for Codex app-server behavior, but the
runtime is rebuilt in Go around PicoClaw's own abstractions and persistence model.

Codex authentication is external to PicoClaw. The fork starts `codex app-server` directly inside an auth-prepared
environment or home and does not preserve PicoClaw's old OAuth or credential-management framework.

## 2. Goals

### 2.1 Goals

- Preserve PicoClaw's multi-agent workspace model, hooks, skills, cron, memory, and generic channel manager.
- Keep Codex behind PicoClaw abstractions rather than creating a separate host runtime.
- Make PicoClaw session history the source of truth for recovery and persistence.
- Stream only final assistant text to Telegram and Discord.
- Keep full PicoClaw tool support, including PicoClaw-managed MCP, through the Codex app-server bridge.
- Reduce config, provider, auth, and channel surface to the minimum needed for this fork.

### 2.2 Non-goals

- Preserve backward compatibility for existing config or session metadata formats.
- Preserve the web launcher, OAuth UI, or generic multi-provider catalog.
- Support websocket app-server transport in v1.
- Use Codex-native MCP management in v1.

## 3. Locked Product Decisions

- Codex app-server stays behind PicoClaw's abstraction layer.
- PicoClaw local session state is authoritative; Codex thread state is an execution mirror.
- Prompt/context assembly stays close to the current PicoClaw shape.
- Codex tool execution is adapted onto `pkg/tools` and `pkg/isolation`.
- The fork is unapologetically Codex-first. Old provider/model/config machinery is removed rather than carried in
  compatibility mode.
- `web/` and launcher surfaces are removed.
- Agent files, hooks, skills, cron, memory, MCP, and the generic channel manager remain.
- Only Telegram and Discord channels remain compiled in.
- Each `(channel thread, agent id)` pair gets its own Codex thread binding.
- Cron/background runs always start a fresh Codex thread, but still append to the agent's normal local session history.
- AGENT frontmatter `model` remains a per-agent default.
- AGENT frontmatter `tools`, `skills`, and `mcpServers` become strict allowlists.
- PicoClaw-managed MCP stays; Codex-native MCP is disabled.
- MCP discovery/deferred loading and large-payload artifact behavior stay.
- DeepSeek fallback is narrow: only startup/connect/resume failure and usage exhaustion trigger automatic fallback.

## 4. Architecture Overview

## 4.1 High-level shape

The fork keeps the existing agent loop as the orchestration layer, but adds a Codex-specific runtime capability behind
the provider boundary.

```text
Telegram / Discord
        |
   Channel Manager
        |
     Agent Loop
        |
  +-----+--------------------+
  |                          |
Codex runtime path      DeepSeek fallback path
  |                          |
pkg/codexruntime        pkg/providers/openai_compat
  |                          |
codex app-server         HTTPS API
```

The important boundary is:

- Codex remains selected and owned through PicoClaw's model/provider flow.
- The generic request-response provider path remains for DeepSeek.
- Codex app-server uses an extended provider capability because plain `Chat()` is not rich enough for thread binding,
  native compaction, approvals, and tool-call streaming.

## 4.2 New package plan

### Keep

- `pkg/agent`
- `pkg/session`
- `pkg/tools`
- `pkg/isolation`
- `pkg/mcp`
- `pkg/memory`
- `pkg/cron`
- `pkg/channels`
- `pkg/commands`
- `pkg/skills`

### Add

- `pkg/codexruntime`
  - `protocol.go`: version-locked app-server request, response, and notification types
  - `client.go`: persistent `stdio` JSON-RPC client, initialize handshake, timeout and restart handling
  - `catalog.go`: lazy `model/list` with bundled fallback models
  - `binding_store.go`: disk persistence for per-thread Codex bindings and runtime settings
  - `projector.go`: canonical event projection and final-response selection
  - `tool_bridge.go`: bridge between Codex dynamic tool calls and `pkg/tools`
  - `approval_bridge.go`: permanent-YOLO approval responder inside PicoClaw policy limits
  - `runner.go`: start/resume/interrupt/compact attempt execution
  - `status.go`: status payloads for CLI and channel commands

### Simplify

- `pkg/providers`
  - keep a small base interface surface
  - keep DeepSeek fallback provider
  - replace `codex exec` provider with a Codex app-server backed provider capability

### Remove

- `web/`
- launcher commands and launcher APIs
- `pkg/auth`
- old OAuth and credential management surfaces
- unused channels
- unused providers and model registry branches

## 5. Provider Boundary

## 5.1 Why the old provider interface is not enough

The current `providers.LLMProvider` contract is stateless request-response:

- `Chat(...)`
- optional `ChatStream(...)`

That contract works for HTTP providers like DeepSeek, but not for Codex app-server because the Codex runtime needs:

- persistent thread binding
- async event projection
- native tool-call handling
- approval handling
- native compaction
- thread status and control commands

## 5.2 New capability model

The provider package should keep the current base interface for stateless models, but add a Codex-specific capability
interface for interactive runtimes. The agent loop checks for that capability and uses it only for Codex-backed agents.

Design intent:

- DeepSeek continues to use the existing LLM request path.
- Codex uses a richer turn runner without bypassing PicoClaw's provider selection boundary.

This preserves the user's requested architecture: Codex stays behind PicoClaw abstractions, but PicoClaw no longer
pretends Codex is just another HTTP completion endpoint.

## 6. Session and Thread Model

## 6.1 Source of truth

PicoClaw's local session history remains canonical.

Codex thread state is a live execution mirror that may be resumed, discarded, or reseeded from PicoClaw history.

This means:

- recovery decisions are made from PicoClaw session state first
- Codex bindings are durable but disposable
- DeepSeek fallback always has a usable local transcript even when Codex thread state is lost

## 6.2 Binding key

Each Codex thread binding is keyed by:

- channel identity
- thread/chat/topic identity
- agent id

This allows one Telegram or Discord conversation to switch agents without mixing Codex threads.

## 6.3 Binding contents

The binding store should persist:

- `thread_id`
- `agent_id`
- `channel`
- `thread_key`
- `model`
- `thinking_mode`
- `fast_enabled`
- `created_at`
- `updated_at`
- `last_user_message_at`
- runtime metadata needed for restart and compaction bookkeeping

The exact on-disk layout can be fork-native. Compatibility with current PicoClaw metadata is not required.

## 6.4 Runtime settings

Per-thread runtime settings are persisted and survive restarts:

- active model
- active thinking mode
- `fast` toggle

`/reset` clears only the Codex conversation thread. It does not clear these runtime settings.

## 6.5 Recovery policy

If the app-server fails mid-run or on resume:

1. restart the app-server once
2. attempt `thread/resume` once
3. if resume still fails, create a fresh Codex thread
4. seed that fresh thread from the last 3 turns from PicoClaw session history

Normal restarts and channel reconnects should resume the existing binding whenever possible.

## 7. Context and Agent Semantics

## 7.1 Context assembly

The current PicoClaw context builder remains the main context source.

The Codex port should not replace PicoClaw's session, memory, hook, or workspace prompt assembly with an OpenClaw-style
transcript-first model. Instead:

- PicoClaw builds the effective prompt and history
- Codex receives that state through the app-server turn model
- Codex thread persistence accelerates and stabilizes execution, but does not replace local transcript ownership

## 7.2 Agent frontmatter

The fork should make AGENT frontmatter more enforceable:

- `model`: per-agent default Codex model
- `tools`: strict allowlist
- `skills`: strict allowlist
- `mcpServers`: strict allowlist

Current parsing already exists for `model`, `skills`, and `mcpServers`. The fork should make these fields operational
rather than informational.

## 7.3 Cron and background runs

Cron jobs and background runs do not reuse the bound interactive Codex thread.

Each scheduled run gets:

- a fresh Codex thread
- the agent's normal workspace and tool environment
- the same local session log append behavior as interactive runs

## 8. Tool, MCP, and Approval Model

## 8.1 PicoClaw remains the tool host

Codex app-server does not become the primary tool system.

Instead, PicoClaw remains the owner of:

- local tools in `pkg/tools`
- subprocess isolation in `pkg/isolation`
- MCP server lifecycle in `pkg/mcp`
- session logging of tool results

The Codex tool bridge translates Codex app-server tool requests into PicoClaw tool executions and returns structured
results back into the live Codex turn.

## 8.2 MCP stays PicoClaw-managed

PicoClaw's existing MCP subsystem remains first-class:

- MCP servers are configured under PicoClaw config
- PicoClaw starts or connects to MCP servers itself
- discovered MCP tools are wrapped into normal PicoClaw tools
- Codex sees them through the same PicoClaw tool bridge as any other tool

Codex-native MCP management is disabled in v1.

This keeps one tool universe, one config surface, and one fallback-compatible behavior model.

## 8.3 MCP allowlists

`mcpServers` becomes a strict per-agent allowlist.

Practical effect:

- only listed MCP servers are registered for that agent
- cron/background runs inherit only the MCP servers that the target agent explicitly allows
- discovery and deferred loading remain unchanged inside that per-agent scope

## 8.4 Large payload behavior

The existing MCP artifact policy remains:

- oversized text is written to workspace artifacts and referenced by path
- binary/media results are stored as local media artifacts
- model context receives only a compact note plus the artifact handle

This is a good fit for Codex because it keeps the runtime lean and avoids blowing context on machine payloads.

## 8.5 Tool failure loop guard

If a PicoClaw tool call fails during a Codex turn:

- return the failure result to Codex
- let Codex attempt recovery in the same turn
- stop the turn only after too many tool failures in a row

The exact threshold can be a small config value with a hard default in the runtime.

## 8.6 Approvals

Native app-server approvals remain part of the protocol surface, but the fork runs with effectively permanent YOLO
behavior inside PicoClaw's sandbox and isolation constraints.

That means:

- approval requests are auto-accepted where PicoClaw policy allows the action
- PicoClaw sandbox and isolation rules still apply
- there is no interactive approval UX in v1

## 9. Event Projection and Streaming

The event projector is the most important part of the port.

Rules:

- treat `item/*` notifications as canonical streamed state
- ignore reasoning items for user-visible output
- ignore transient plan/progress chatter for user-visible output
- surface only final assistant text to Telegram and Discord
- preserve structured internal state for logging and status

User-visible streaming should therefore behave like this:

- the channel sees assistant text accumulate
- the channel does not see reasoning
- the channel does not see app-server internal progress messages
- the channel does not see compaction chatter

CLI and admin status may expose richer Codex runtime internals for debugging.

## 10. Commands and Operational Surface

No web or launcher control surface survives in v1.

Operational control lives in generic PicoClaw commands surfaced by channels and CLI.

Required command set:

- `/set model`
- `/set thinking`
- `/fast`
- `/reset`
- `/compact`
- `status`

Behavior:

- `/set model`: allow any model reported by the app-server catalog
- `/set thinking`: allow any thinking mode reported for the selected model
- `/fast`: toggle Codex native fast mode on the current bound thread
- `/compact`: manually trigger native Codex compaction
- `/reset`: clear only the Codex conversation thread
- `status`: expose Codex runtime internals, including bound thread id, current model, thinking mode, fast state, and
  compaction/recovery state

These command handlers should be generic command definitions in `pkg/commands`, then registered by Telegram and Discord
rather than being hardcoded only in one channel package.

The CLI/admin control surface should stay minimal and Codex-specific:

- model list
- runtime status
- thread reset
- manual compact

## 11. Model Discovery and Fallback

## 11.1 Discovery

Codex model discovery is lazy and best-effort:

- use `model/list` on demand
- if discovery fails, fall back to a bundled static list
- cache only as needed for runtime efficiency

The bundled fallback list should at least include:

- `gpt-5.4`
- `gpt-5.4-mini`

The catalog also carries Codex-reported thinking and speed-tier metadata so channel commands can validate runtime
changes without hardcoding a stale enum.

## 11.2 Primary and fallback model model

The new config surface is intentionally narrow:

- primary runtime: Codex
- fallback runtime: DeepSeek

There is no general `model_list` or multi-provider matrix anymore.

Per-agent and per-thread variation still exists, but only inside the Codex-first model:

- global runtime defaults
- AGENT frontmatter default model
- per-thread runtime overrides

## 11.3 DeepSeek fallback

DeepSeek remains a plain request-response fallback path.

Automatic fallback should happen only when:

- Codex cannot start
- Codex cannot connect
- Codex cannot resume a thread
- Codex reports usage exhaustion

Fallback should not trigger for every ordinary Codex turn error. Outside the narrow cases above, the user should switch
fallback intentionally.

## 12. Compaction Policy

Compaction is native Codex compaction only.

Rules:

- trigger automatically when context is low
- run only between turns
- do not interrupt active turns for compaction
- preserve a manual `/compact` control path

The initial runtime default can use a simple threshold, for example triggering when roughly 30% context remains.

Compaction policy belongs in agent/session management, not buried inside the provider implementation, so the behavior is
visible and controllable at the orchestration layer.

## 13. Channel and UI Scope

Keep:

- generic channel manager
- Telegram
- Discord

Remove:

- Matrix
- IRC
- Pico channel
- other unused channel implementations
- web launcher and backend/frontend control surfaces

This keeps channel architecture stable while sharply reducing maintenance and dependency surface.

## 14. Config Rewrite

The fork should replace the current broad provider-oriented schema with a smaller runtime-oriented schema.

Example shape:

```json
{
  "runtime": {
    "primary": "codex",
    "fallback": "deepseek",
    "codex": {
      "command": "codex",
      "args": ["app-server", "--listen", "stdio://"],
      "request_timeout_ms": 60000,
      "auto_compact_threshold_percent": 30
    },
    "deepseek": {
      "model": "deepseek-chat",
      "api_base": "https://api.deepseek.com/v1",
      "api_key": "env:DEEPSEEK_API_KEY"
    }
  }
}
```

The rest of the existing config tree survives only where it still maps to the retained system:

- agents
- channels
- tools
- cron
- memory
- workspace
- hooks
- logging

Remove config branches tied only to removed providers, removed channels, removed auth flows, and removed launcher
surfaces.

## 15. Deletion Map

First-pass deletion targets:

- `web/`
- launcher commands and launcher-related backend code
- `pkg/auth`
- OAuth and credential command surfaces
- unused channel packages under `pkg/channels`
- unused provider packages under `pkg/providers`
- old model factory and default model catalog branches
- config defaults and docs that exist only for removed providers/channels/UI

Likely keep, but simplify:

- `pkg/providers/openai_compat`
- `pkg/providers/protocoltypes`
- `pkg/providers/types.go`

The exact cut list should be done after the Codex runtime package skeleton exists, so imports can be trimmed in one pass
instead of churned repeatedly.

## 16. Implementation Order

1. Add `pkg/codexruntime` with transport, protocol, projector, binding, and discovery skeletons.
2. Introduce the Codex provider capability surface and wire the agent loop to use it.
3. Persist per-thread runtime settings and Codex bindings in the session layer.
4. Port the tool bridge, approval bridge, and native compaction flow.
5. Add generic commands for model, thinking, fast, compact, reset, and status.
6. Enforce AGENT allowlists for tools, skills, and `mcpServers`.
7. Simplify config to Codex primary plus DeepSeek fallback.
8. Remove old providers, auth, launcher, and unused channels.
9. Update docs and example config to match the new fork.

## 17. Summary

This fork keeps PicoClaw's strong parts: agent structure, tools, MCP, memory, cron, and channels.

What changes is the runtime core:

- Codex app-server becomes the primary execution engine
- PicoClaw session state remains authoritative
- DeepSeek becomes a narrow emergency fallback
- provider/config/auth/channel sprawl is removed

The result should feel like a lean Codex-native system built on PicoClaw's backbone, not a generic multi-provider bot
that happens to support Codex.
