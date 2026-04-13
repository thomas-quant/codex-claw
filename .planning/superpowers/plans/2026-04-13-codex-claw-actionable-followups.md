# Codex-Claw Actionable Followups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the implementable followups by deduplicating tool execution, hard-deprecating legacy fallback config, polishing continuity/status output, and cleaning active docs/examples.

**Architecture:** Keep the current Codex-first runtime shape intact. This plan is a cleanup pass, not a redesign: `pkg/agent` remains the orchestration layer, `pkg/codexruntime` remains the continuity/runtime layer, and `pkg/config` remains a hard-broken Codex-first schema with clearer deprecation behavior around legacy fallback fields.

**Tech Stack:** Go, Cobra command helpers, Markdown docs, `go test`

---

## File Structure

### Create

- `pkg/agent/tool_exec_shared.go` — shared helper(s) for tool lookup/execution/result shaping used by both interactive and legacy paths

### Modify

- `pkg/agent/interactive_tool_exec.go`
- `pkg/agent/loop.go`
- `pkg/agent/loop_test.go`
- `pkg/agent/instance.go`
- `pkg/agent/instance_test.go`
- `pkg/agent/registry_test.go`
- `pkg/config/config.go`
- `pkg/config/config_test.go`
- `pkg/commands/cmd_runtime_helpers.go`
- `pkg/commands/phase3_commands_test.go`
- `pkg/audio/asr/README.md`
- `pkg/audio/tts/README.md`
- `docs/configuration.md`
- `docs/providers.md`
- `docs/troubleshooting.md`
- `docs/tools_configuration.md`
- `docs/security_configuration.md`
- `docs/cron.md`
- `docs/spawn-tasks.md`
- `docs/subturn.md`
- `followups.md`

### Keep As-Is

- Codex app-server protocol/runtime behavior
- thread rollover summary policy
- live Codex context-signal compaction design
- voice runtime reintroduction

## Task 1: Deduplicate Tool Execution Paths

**Files:**
- Create: `pkg/agent/tool_exec_shared.go`
- Modify: `pkg/agent/interactive_tool_exec.go`
- Modify: `pkg/agent/loop.go`
- Modify: `pkg/agent/loop_test.go`

- [ ] Extract the shared pieces of tool execution from the interactive path and the legacy path into one helper file:
  - hook `BeforeTool` / `ApproveTool` / `AfterTool`
  - `tools.WithToolInboundContext(...)`
  - async follow-up publication
  - `ForUser`, `ResponseHandled`, media, artifact-tag, and sensitive-filter handling
  - tool-result session/history persistence
- [ ] Keep the caller-specific differences outside the shared helper:
  - interactive path still returns `providers.InteractiveToolResult`
  - legacy path still appends `providers.Message` tool results into the normal loop
- [ ] Update tests so both paths still prove the same behavior for:
  - successful tool execution
  - hook denial / approval denial
  - async tool follow-up publication
  - handled media/file responses
- [ ] Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestAgentLoop_InteractiveProviderToolCallbackUsesAgentToolExecution|TestProcessMessage_CommandOutcomes' -count=1
```

## Task 2: Hard-Deprecate Legacy Fallback Fields

**Files:**
- Modify: `pkg/config/config.go`
- Modify: `pkg/config/config_test.go`
- Modify: `pkg/agent/instance.go`
- Modify: `pkg/agent/instance_test.go`
- Modify: `pkg/agent/registry_test.go`
- Modify: `docs/configuration.md`
- Modify: `docs/providers.md`
- Modify: `docs/troubleshooting.md`

- [ ] Decide the deprecation behavior in code and make it explicit:
  - `model_fallbacks`
  - `image_model_fallbacks`
  - structured `model.fallbacks`
  - structured `subagents.model.fallbacks`
- [ ] Recommended implementation: parse them, ignore them, and surface a warning or explicit test-backed note instead of silently keeping fallback chains alive.
- [ ] Remove agent/runtime assumptions that these fields drive the active fallback chain. The only automatic runtime fallback should remain Codex -> DeepSeek under the narrow Phase 8 conditions.
- [ ] Update tests that currently expect inherited fallback arrays so they instead assert deprecation/ignore behavior.
- [ ] Update active docs to say fallback behavior is runtime-owned and not configured through old model fallback arrays.
- [ ] Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/config ./pkg/agent ./pkg/commands -count=1
```

## Task 3: Polish Runtime Status And Continuity Output

**Files:**
- Modify: `pkg/commands/cmd_runtime_helpers.go`
- Modify: `pkg/commands/phase3_commands_test.go`
- Modify: `docs/troubleshooting.md` if command output wording changes
- Modify: `followups.md`

- [ ] Improve status/help output using continuity data that already exists today:
  - `last_user_message_at`
  - `last_compaction_at`
  - current recovery state wording
  - `force_fresh_thread` only if it is genuinely useful to operators
- [ ] Keep this to display/wording cleanup. Do not add new runtime policy or new state.
- [ ] Make command output stable and concise enough that tests can assert it without brittle timestamp formatting.
- [ ] Remove the corresponding continuity/status bullet from `followups.md` only if this task fully closes it.
- [ ] Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/commands -run 'Test(Runtime_|Phase3_)' -count=1
```

## Task 4: Sweep Active Docs And Examples

**Files:**
- Modify: `pkg/audio/asr/README.md`
- Modify: `pkg/audio/tts/README.md`
- Modify: `docs/configuration.md`
- Modify: `docs/providers.md`
- Modify: `docs/tools_configuration.md`
- Modify: `docs/security_configuration.md`
- Modify: `docs/troubleshooting.md`
- Modify: `docs/cron.md`
- Modify: `docs/spawn-tasks.md`
- Modify: `docs/subturn.md`
- Modify: `followups.md`

- [ ] Rewrite the ASR/TTS READMEs so they describe the current fork honestly:
  - remove `model_list`
  - remove provider-matrix assumptions
  - describe current voice docs as legacy/conditional where appropriate
- [ ] Sweep the surviving English docs for stale references to:
  - launcher/web setup
  - migration/import surfaces
  - removed channels (`whatsapp`, `slack`, `feishu`, `matrix`, etc.) used as live examples
  - OAuth/provider-era auth wording that no longer describes the fork
- [ ] Keep historical design/spec docs out of scope; only clean active operator/developer docs and obvious user-facing comments/help text.
- [ ] Remove closed bullets from `followups.md` and leave only the strategy-blocked or intentionally deferred items.
- [ ] Run a targeted stale-term sweep:

```bash
rg -n 'model_list|providers|oauth|launcher|migration|whatsapp|slack|feishu|matrix' \
  pkg/audio/asr/README.md pkg/audio/tts/README.md docs
```

- [ ] Then run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/audio/asr ./pkg/audio/tts ./pkg/agent ./pkg/config ./pkg/commands -count=1
```

## Task 5: Close-Out Verification

**Files:**
- Modify any touched files from Tasks 1-4

- [ ] Run the focused package sweep:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent ./pkg/config ./pkg/commands ./pkg/audio/asr ./pkg/audio/tts -count=1
```

- [ ] Run the broader guardrail sweep:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./... -count=1
```

- [ ] Review any failures for actual regressions introduced by this pass. Fix only those regressions; do not reopen blocked memory or rename work.
- [ ] Confirm `followups.md` now contains only:
  - summary/handoff strategy work
  - live Codex context-signal compaction decisions
  - voice-runtime reintroduction if still desired
  - any genuinely new debt discovered during this pass

## Self-Review

- Spec coverage:
  - tool dedupe -> Task 1
  - fallback-surface cleanup -> Task 2
  - low-risk status polish -> Task 3
  - docs/example cleanup -> Task 4
  - verification -> Task 5
- Placeholder scan: no `TODO`/`TBD` placeholders remain; all tasks have concrete files and commands.
- Type consistency:
  - tool dedupe is centered in `pkg/agent`, not spread into provider/runtime packages
  - fallback deprecation stays in config/agent/docs rather than changing the Codex runtime contract
  - status polish is limited to command formatting and tests
