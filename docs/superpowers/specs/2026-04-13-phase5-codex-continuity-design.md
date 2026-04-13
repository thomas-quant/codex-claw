# Phase 5: Codex Continuity And Native Compaction Design

## Goal

Make the Codex-first runtime behave like a durable conversation system instead of a thin turn transport. Phase 5 adds thread bootstrap from PicoClaw-owned history, inactivity rollover, resume recovery, and proactive native compaction without changing the chosen architecture: PicoClaw session state remains authoritative and Codex thread state remains an execution mirror.

## Scope

This phase adds policy for:

- fresh-thread bootstrap from local session history
- restart and resume recovery
- 8-hour inactivity rollover
- proactive native compaction between turns
- richer runtime status metadata for continuity decisions

This phase does not add:

- a new memory subsystem
- handoff-summary generation beyond current local history usage
- media-input continuity
- cross-channel or cross-agent thread sharing
- config cleanup beyond any minimal runtime fields needed for continuity policy

## Recommended Approach

Use agent/session-owned lifecycle policy.

Alternatives considered:

1. Provider-owned lifecycle policy. This hides important behavior inside the Codex transport path and conflicts with the decision that PicoClaw state is the source of truth.
2. Split policy between agent and provider. This would make recovery and bootstrap decisions harder to reason about and easier to regress.
3. Agent/session-owned lifecycle policy. Recommended.

Under the recommended approach:

- `pkg/codexruntime` owns protocol primitives, thread binding persistence, and direct operations such as resume, start, compact, and status reads
- `pkg/providers/codex_app_server_provider.go` stays a transport adapter
- `pkg/agent` decides when to reuse, roll over, compact, reseed, or recover a Codex thread

## Fresh-Thread Bootstrap

The current interactive provider path only forwards the last user message. That is acceptable only when a durable Codex thread already contains the prior conversation. It is not acceptable for:

- first use in a thread
- recovery after failed resume
- post-rollover fresh threads

Phase 5 changes fresh-thread creation so the first turn sent to a new Codex thread is seeded from PicoClaw-owned context.

The bootstrap payload should be lean and deterministic:

- existing agent/system/bootstrap context already assembled by PicoClaw
- the recent local session transcript needed for continuity
- the current user message

Do not introduce a new summary step in this phase. The initial continuity path should use recent local turns directly, with recovery fallback limited to the last 3 local turns when a previously bound thread cannot be resumed.

## Recovery Policy

Recovery remains bounded and predictable.

On a normal turn with an existing binding:

1. try to resume the bound Codex thread
2. if the app-server connection is dead, restart it once
3. try resume once more
4. if resume still fails, create a fresh Codex thread seeded from the last 3 local turns plus the current user turn

This policy is not a generic retry loop. It is a single restart plus single resume retry, then a deterministic fallback to a new thread.

The binding metadata should record enough state for status and debugging:

- last successful user-turn timestamp
- last recovery path used
- last compaction timestamp
- whether the current turn forced a fresh thread after failed resume

## Inactivity Rollover

Codex threads should survive restarts and reconnects, but not sit indefinitely.

Before starting a new user turn, the agent/session layer should compare the current time to the binding's `last_user_message_at`. If inactivity exceeds 8 hours, the next user turn should use a fresh Codex thread instead of attempting resume.

The fresh thread should be seeded from local PicoClaw context rather than the expired Codex thread. For phase 5, use recent local transcript turns. A richer handoff-summary policy can come later.

Rollover is per bound `(channel thread, agent id)` scope. It does not affect other agents in the same chat and does not clear per-thread runtime settings like model, thinking mode, or `fast`.

## Proactive Native Compaction

Native Codex compaction should become a first-class continuity tool, not only an error-recovery path.

Policy:

- read `runtime.codex.auto_compact_threshold_percent`
- if the interactive Codex thread is low on context, compact between turns
- never compact mid-turn
- preserve the existing generic context-management behavior for non-Codex paths

The compact decision belongs in `pkg/agent`, because it is session policy. The actual compact operation belongs in the interactive Codex runtime/provider surface.

The first pass does not need sophisticated prediction. It only needs a stable threshold-driven decision using available runtime/context signals and clear bookkeeping in thread status.

## Runtime Status Contract

Phase 5 needs richer status than the current `thread_id/model/thinking/fast` snapshot.

The runtime status used by the agent loop should expose:

- bound thread id
- effective model
- effective thinking mode
- `fast` enabled state
- `last_user_message_at`
- last compaction timestamp
- last recovery state
- whether the next turn should force a fresh thread

This is primarily for orchestration, not user-facing verbosity. CLI and channel status commands can surface a subset or all of it later.

## Responsibilities By Layer

### `pkg/codexruntime`

- binding-store persistence for continuity metadata
- protocol/client primitives for resume, start, compact, and status
- small helpers for bootstrap or recovery requests if needed

### `pkg/providers`

- pass agent/session continuity instructions into the runtime
- remain transport-focused
- avoid owning inactivity or compaction policy

### `pkg/agent`

- decide whether to resume or force a fresh thread
- decide when to compact
- assemble bootstrap seed input for new threads
- update continuity metadata after successful user turns

## Testing Strategy

Keep verification narrow and policy-focused.

Required coverage:

- new-thread bootstrap uses local session context instead of only the last user message
- resume failure performs one restart, one retry, then fresh-thread seed from last 3 turns
- inactivity greater than 8 hours forces fresh-thread rollover
- runtime settings survive rollover and `/reset`
- proactive native compaction triggers only between turns
- non-Codex paths remain unchanged

Touched-package verification should stay within:

- `./pkg/codexruntime`
- `./pkg/providers`
- `./pkg/agent`

## Risks

- duplicating too much context when both PicoClaw and Codex thread state are warm
- putting too much bootstrap logic into the provider layer
- over-triggering compaction from weak heuristics
- accidental coupling between continuity policy and later command/config cleanup

The mitigation is to keep the bootstrap policy lean, keep the retry policy bounded, and keep the orchestration boundary explicit: session policy in `pkg/agent`, operations in `pkg/codexruntime`.
