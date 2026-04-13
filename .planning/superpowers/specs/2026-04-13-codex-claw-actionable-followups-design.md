# Codex-Claw Actionable Followups Design

## Goal

Close the followups that are implementable now without reopening unresolved product decisions around memory, rollover summaries, or voice reintroduction.

This pass is intentionally narrow. It should improve runtime clarity and repo consistency without creating a new architecture phase.

## Included Scope

### 1. Tool execution cleanup

`pkg/agent` currently maintains an interactive Codex tool-execution path and a legacy non-interactive path with overlapping behavior. This pass should factor the shared execution/result-shaping logic into one internal helper so both paths use the same rules for:

- tool lookup and permission checks
- tool execution
- result shaping and session logging
- error returns back to the provider loop

The goal is deduplication, not a behavior rewrite.

### 2. Legacy fallback cleanup

The runtime contract is now Codex primary with DeepSeek automatic fallback only in narrow cases. Old `model_fallbacks` config and frontmatter language should no longer appear to be a supported first-class policy surface.

This pass should either remove that legacy surface where safe or hard-deprecate it with explicit warnings and docs cleanup. The end state must make the real fallback contract obvious.

### 3. Runtime-facing continuity cleanup

Low-risk continuity/status polish is in scope if it improves operator understanding without introducing new policy:

- expose already-known continuity metadata more cleanly in status/help text
- tighten recovery-state wording if it is misleading today

This does not include new memory behavior, summary generation, or new rollover semantics.

### 4. Docs and example cleanup

Bring surviving docs in line with the fork boundary:

- rewrite ASR/TTS docs to remove `model_list` and provider-era setup
- remove stale launcher, migration, removed-channel, and OAuth-era references where they remain in active docs/examples
- replace removed channel names used as generic examples when they no longer match the fork
- sweep comments/help text for obviously stale provider/auth terminology

Localized docs are already mostly gone. This pass should only touch surviving docs and examples.

## Explicitly Out Of Scope

- rollover handoff summaries
- replacing transcript bootstrap with summary-first bootstrap
- live Codex-reported context-signal compaction
- voice runtime reintroduction
- repo-wide identity rename and module/binary renaming

Those remain separate decisions or separate cleanup work.

## Recommended Approach

Use one followup branch of work with three implementation slices:

1. runtime deduplication in `pkg/agent`
2. fallback-surface cleanup in config/frontmatter/help text
3. docs/example sweep

That keeps behavior changes isolated from docs churn and avoids mixing blocked memory work into the same pass.

## Success Criteria

- shared tool execution logic lives in one place instead of duplicated interactive/legacy branches
- fallback behavior and supported config surface are unambiguous in code and docs
- active docs/examples no longer advertise removed provider/channel/auth surfaces
- existing runtime behavior stays green under targeted and package-level Go tests
