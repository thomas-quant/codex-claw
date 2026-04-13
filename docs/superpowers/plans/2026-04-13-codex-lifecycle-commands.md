# Codex Lifecycle And Runtime Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first real Codex runtime control surface: model discovery, persisted per-thread runtime settings, native compaction and recovery policy, and the `/set`, `/fast`, `/compact`, `/reset`, and `status` commands.

**Architecture:** Keep Codex behind PicoClaw's provider abstraction, but move lifecycle policy into agent/session management instead of burying it in `pkg/codexruntime`. `pkg/codexruntime` owns protocol/client/catalog/control primitives; `pkg/agent` decides when to compact, how to recover, and how command changes affect the current bound thread.

**Tech Stack:** Go 1.25, Codex app-server JSON-RPC over `stdio`, existing `pkg/commands`, `pkg/agent`, `pkg/session`, `go test`

---

## Carry-Over Followups From Phases 1-2

- The current Codex provider only forwards the last user message in [pkg/providers/codex_app_server_provider.go](/root/picoclaw/pkg/providers/codex_app_server_provider.go:26). New-thread bootstrap and recovery seeding still need a real session-history handoff instead of pretending the bound thread already has context.
- [pkg/codexruntime/runner.go](/root/picoclaw/pkg/codexruntime/runner.go:38) still treats any resume failure as "start a new thread." Phase 3 needs a classified recovery path instead of that blanket fallback.
- [pkg/codexruntime/binding_store.go](/root/picoclaw/pkg/codexruntime/binding_store.go:16) already stores `model`, `thinking_mode`, `fast_enabled`, and `last_user_message_at`, but there is no public API yet for updating them independently or surfacing a thread status snapshot.
- The current `/clear` command still owns the `/reset` alias through [pkg/commands/cmd_clear.go](/root/picoclaw/pkg/commands/cmd_clear.go:5). That must split: `/clear` stays local-history cleanup, `/reset` becomes Codex-thread reset.
- Auto-compaction in [pkg/agent/loop.go](/root/picoclaw/pkg/agent/loop.go:1778) and retry compaction in [pkg/agent/loop.go](/root/picoclaw/pkg/agent/loop.go:2244) still call the generic context manager directly. Codex-native compaction has to be orchestrated above the provider path.
- `pkg/commands/cmd_switch.go`, `cmd_show.go`, and `cmd_list.go` are still config-centric. They do not know about Codex-native model discovery, thinking modes, or the `fast` toggle.
- The interactive tool-execution helper in [pkg/agent/interactive_tool_exec.go](/root/picoclaw/pkg/agent/interactive_tool_exec.go:1) duplicates part of the legacy tool path. That cleanup is not required for phase 3, but keep it on the backlog while touching adjacent loop code.

## File Structure

### Create

- `pkg/codexruntime/catalog.go` — lazy `model/list` support with bundled fallback models and thinking metadata
- `pkg/codexruntime/catalog_test.go` — discovery success, cache, and fallback-list coverage
- `pkg/codexruntime/status.go` — runtime-facing thread status snapshot types
- `pkg/codexruntime/status_test.go` — status snapshot and binding projection coverage
- `pkg/commands/cmd_set.go` — `/set model` and `/set thinking`
- `pkg/commands/cmd_fast.go` — `/fast`
- `pkg/commands/cmd_compact.go` — `/compact`
- `pkg/commands/cmd_status.go` — Codex runtime `status`
- `pkg/commands/cmd_reset.go` — Codex-thread reset distinct from `/clear`

### Modify

- `pkg/codexruntime/protocol.go`
- `pkg/codexruntime/client.go`
- `pkg/codexruntime/client_test.go`
- `pkg/codexruntime/runner.go`
- `pkg/codexruntime/runner_test.go`
- `pkg/codexruntime/binding_store.go`
- `pkg/codexruntime/binding_store_test.go`
- `pkg/providers/types.go`
- `pkg/providers/codex_app_server_provider.go`
- `pkg/providers/codex_app_server_provider_test.go`
- `pkg/agent/loop.go`
- `pkg/agent/loop_test.go`
- `pkg/agent/context_manager.go`
- `pkg/commands/runtime.go`
- `pkg/commands/builtin.go`
- `pkg/commands/cmd_clear.go`
- `pkg/commands/cmd_switch.go`
- `pkg/commands/cmd_show.go`
- `pkg/commands/cmd_list.go`
- `pkg/commands/*_test.go` for the touched command files

## Task 1: Add Codex Catalog, Binding Mutators, And Control Primitives

**Files:**
- Create: `pkg/codexruntime/catalog.go`
- Create: `pkg/codexruntime/catalog_test.go`
- Create: `pkg/codexruntime/status.go`
- Create: `pkg/codexruntime/status_test.go`
- Modify: `pkg/codexruntime/protocol.go`
- Modify: `pkg/codexruntime/client.go`
- Modify: `pkg/codexruntime/client_test.go`
- Modify: `pkg/codexruntime/binding_store.go`
- Modify: `pkg/codexruntime/binding_store_test.go`

- [ ] Add protocol types and client methods for `model/list` and native compaction start.
- [ ] Introduce a small model catalog cache with bundled fallback models `gpt-5.4` and `gpt-5.4-mini`.
- [ ] Extend the binding store with explicit mutators for model, thinking mode, fast toggle, last-user-message time, and binding deletion.
- [ ] Add a runtime status snapshot that merges binding metadata with live client-known state for command responses.
- [ ] Keep verification narrow:

Run:

```bash
/tmp/go/bin/go test ./pkg/codexruntime -run 'TestCatalog|TestBindingStore|TestClient' -count=1
```

## Task 2: Move Recovery And Native Compaction Policy Into Agent/Session Flow

**Files:**
- Modify: `pkg/codexruntime/runner.go`
- Modify: `pkg/codexruntime/runner_test.go`
- Modify: `pkg/providers/types.go`
- Modify: `pkg/providers/codex_app_server_provider.go`
- Modify: `pkg/providers/codex_app_server_provider_test.go`
- Modify: `pkg/agent/loop.go`
- Modify: `pkg/agent/loop_test.go`
- Modify: `pkg/agent/context_manager.go`

- [ ] Extend the interactive Codex request path so the loop can pass recovery and control metadata, not just `InputText`, tools, and chunks.
- [ ] Replace the current "resume failed => always start fresh" behavior with:
  - restart app-server once
  - try resume once
  - if resume still fails, seed a fresh thread from the last 3 local turns
- [ ] Update `last_user_message_at` on successful user turns so the later 8-hour rollover policy has real timestamps to work with.
- [ ] Add an orchestration-level native compact hook for interactive Codex providers and call it only between turns, not mid-turn.
- [ ] Keep the generic context manager behavior untouched for non-Codex paths and narrow the first pass to the Codex interactive branch.

Run:

```bash
/tmp/go/bin/go test ./pkg/codexruntime ./pkg/providers ./pkg/agent -run 'TestRunner|TestCodexAppServerProvider|TestAgentLoop' -count=1
```

## Task 3: Add Per-Thread Runtime Commands And Split `/reset` From `/clear`

**Files:**
- Create: `pkg/commands/cmd_set.go`
- Create: `pkg/commands/cmd_fast.go`
- Create: `pkg/commands/cmd_compact.go`
- Create: `pkg/commands/cmd_status.go`
- Create: `pkg/commands/cmd_reset.go`
- Modify: `pkg/commands/runtime.go`
- Modify: `pkg/commands/builtin.go`
- Modify: `pkg/commands/cmd_clear.go`
- Modify: `pkg/commands/cmd_switch.go`
- Modify: `pkg/commands/cmd_show.go`
- Modify: `pkg/commands/cmd_list.go`
- Modify: matching tests under `pkg/commands`

- [ ] Extend `commands.Runtime` with the Codex-specific callbacks phase 3 needs:
  - list models
  - read status
  - set model
  - set thinking
  - toggle fast
  - compact thread
  - reset thread
- [ ] Add `/set model <name>` and `/set thinking <mode>` as the new per-thread runtime commands.
- [ ] Add `/fast` as a sticky toggle stored in the binding.
- [ ] Add `/compact` and `status`.
- [ ] Split `/reset` off from `/clear`; keep `/clear` for local session history and make `/reset` clear only the Codex thread binding plus any live runtime handle.
- [ ] Keep `/switch model`, `/show model`, and `/list models` aligned with runtime truth:
  - either rewire them to Codex-native state
  - or make them thin compatibility shims that point users to the new command surface

Run:

```bash
/tmp/go/bin/go test ./pkg/commands -run 'Test.*(Set|Fast|Compact|Status|Reset|Clear|Switch|Show|List)' -count=1
```

## Task 4: Wire Agent Runtime Callbacks And Persist Settings Per Thread

**Files:**
- Modify: `pkg/agent/loop.go`
- Modify: `pkg/agent/loop_test.go`
- Modify: `pkg/providers/codex_app_server_provider.go`
- Modify: `pkg/providers/types.go`
- Modify: `pkg/codexruntime/binding_store.go`

- [ ] Build the new command callbacks from the current `(session, channel, agent)` scope so they target the bound Codex thread for that exact agent.
- [ ] Persist command-driven model, thinking, and fast changes into the binding store without forcing a new local session.
- [ ] Keep the same Codex thread when settings change unless the runtime explicitly has to refresh the thread state on the next turn.
- [ ] Make `status` report the bound thread id, current model, thinking mode, fast state, and recovery/compaction markers.
- [ ] Add one narrow loop test for command-driven model/thinking/fast persistence and one for `/reset` preserving runtime settings while clearing the thread.

Run:

```bash
/tmp/go/bin/go test ./pkg/agent -run 'TestAgentLoop_.*(Status|Reset|Fast|Set)' -count=1
```

## Task 5: Focused Verification And Follow-On Notes

**Files:**
- Modify only what tasks 1-4 already touched

- [ ] Run only touched-package verification for this phase.
- [ ] Do not widen to `./...` unless a touched-package failure proves cross-package fallout.
- [ ] Capture the next carry-over items at the end of the phase:
  - session-history bootstrap for new threads
  - 8-hour inactivity rollover
  - interactive/legacy tool-path deduplication
  - config rewrite and provider/channel deletion passes

Run:

```bash
/tmp/go/bin/go test ./pkg/codexruntime ./pkg/providers ./pkg/commands ./pkg/agent -count=1
```

## Worker Split

Use disjoint write sets:

1. Runtime worker
   - owns `pkg/codexruntime/*`
   - owns `pkg/providers/codex_app_server_provider.go`
   - owns `pkg/providers/codex_app_server_provider_test.go`

2. Commands worker
   - owns `pkg/commands/*`

3. Agent integration worker
   - owns `pkg/agent/loop.go`
   - owns `pkg/agent/loop_test.go`
   - owns any tiny `pkg/providers/types.go` glue needed by the loop contract

Shared contract between workers:

- bindings remain keyed by `(channel thread, agent id)`
- command changes persist per-thread settings
- `/reset` clears the Codex thread but keeps per-thread runtime settings
- native compaction is orchestrated in agent/session flow, not hidden in the provider
- verification stays package-focused to keep the phase light
