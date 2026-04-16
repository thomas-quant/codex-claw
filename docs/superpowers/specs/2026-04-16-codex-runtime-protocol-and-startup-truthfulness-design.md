# Codex Runtime Protocol And Startup Truthfulness

## Goal

Fix three user-visible runtime problems without widening scope:

1. `codex-claw agent` fails against current `codex app-server` with `401 Authentication Fails (governor)` even when `codex exec` works from the same managed `CODEX_HOME`.
2. gateway startup prints success for disabled services, especially heartbeat.
3. channel startup collapses multiple failure modes into the misleading `No channels enabled` message.

This patch keeps the surviving `gateway` runtime. It does not remove the HTTP runtime, Telegram/Discord support, or health endpoints.

## Root Cause Summary

### 1. Stale app-server protocol client

`pkg/codexruntime` still speaks an older JSON-RPC payload shape:

- `thread/start` expects legacy response fields and only sends a minimal old request body.
- `turn/start` sends `thread_id` and `input_text`.

Current Codex app-server docs use:

- `threadId`
- `input: [{type:"text", text:"..."}]`

The installed `codex` CLI works against the same managed auth state, so the failure is in codex-claw's integration layer rather than auth material.

### 2. Startup text lies about disabled heartbeat

`HeartbeatService.Start()` returns early when disabled, but `pkg/gateway/gateway.go` prints `Heartbeat service started` unconditionally after calling it.

### 3. Channel diagnostics hide the actual block reason

`pkg/channels/manager.go` only initializes Telegram/Discord when:

- channel is enabled
- token is non-empty

If an enabled channel is missing a token or the token is malformed in `.security.yml`, the runtime reports only `No channels enabled`, which is operationally false and hard to debug.

## Chosen Approach

Patch the existing runtime in place.

### Why

- smallest fix set that addresses the observed failures
- preserves current product boundary
- avoids reopening the larger decision of removing the `gateway` runtime entirely
- matches the stated intent of the fork: remove launcher/web UI, keep channel/runtime backbone

## Design

### A. Update `pkg/codexruntime` to current app-server wire shape

Modify request/response structs and client calls so the app-server client uses current field names and payload layout.

Planned changes:

- `thread/start` and `thread/resume`
  - send `approvalPolicy` as before
  - support current `threadId` naming
  - continue accepting legacy response shapes where cheap
- `turn/start`
  - replace legacy `input_text` request body with `input` items
  - send text input as one `text` item
  - use `threadId` rather than `thread_id`
- notification decode structs
  - prefer current camelCase fields where required for turn correlation
  - preserve backward-compatible decoding only where low-cost

The patch stays inside `pkg/codexruntime` and the provider adapter. No behavior change to higher-level agent orchestration beyond making current app-server calls valid.

### B. Make startup reporting truthful

Change gateway startup prints to reflect actual service state.

Planned changes:

- heartbeat:
  - when disabled, print `Heartbeat service disabled`
  - when started, print `Heartbeat service started`
- cron:
  - audit for the same lie pattern
  - if service is always started by design, keep message as-is
  - if disabled/no-op states exist, print the correct state instead

This is a UX/observability fix only.

### C. Expose channel block reasons

Keep existing channel gating logic, but surface why each channel did not initialize.

Planned changes:

- distinguish:
  - disabled in config
  - enabled but token missing
  - factory/init failure
- gateway startup summary should say:
  - enabled channels list when any started
  - otherwise a reasoned summary instead of bare `No channels enabled`

The goal is direct operator diagnosis from startup output without needing source inspection.

## Non-Goals

- removing the `gateway` command or health HTTP server
- changing Telegram/Discord channel behavior beyond diagnostics
- redesigning account/auth management
- broad refactors in gateway/service lifecycle code unrelated to these failures

## Tests

Write failing tests first.

Minimum coverage:

1. `pkg/codexruntime`
   - current `turn/start` request payload uses `threadId` + `input`
   - current `thread/start` request payload matches expected current schema
   - current response decoding still accepts expected returned thread/turn ids
2. `pkg/gateway`
   - disabled heartbeat does not print `started`
   - enabled heartbeat still prints started
3. `pkg/channels` or `pkg/gateway`
   - enabled Telegram without token surfaces a missing-token reason
   - pure disabled-channel config still surfaces the correct disabled state

## Risks

- app-server schema drift beyond the currently documented field names
- tests tied too tightly to log strings rather than behavior
- touching protocol structs may affect older local Codex versions

Mitigation:

- use current official app-server docs as source of truth
- keep compatibility decoding where trivial
- keep the patch narrow

## Verification

After implementation:

- targeted Go tests for `pkg/codexruntime`, `pkg/gateway`, `pkg/channels`
- manual smoke:
  - `codex-claw agent --model gpt-5.4-mini -m "test"`
  - `codex-claw gateway`
  - confirm disabled heartbeat prints disabled
  - confirm malformed/missing Telegram token prints actionable reason
