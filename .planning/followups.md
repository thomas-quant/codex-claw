# Followups

Detailed triage now lives in:

- `.planning/triage/runtime-and-reliability.md`
- `.planning/triage/operator-surface-and-branding.md`
- `.planning/triage/platform-capabilities-and-deferred-work.md`

## Codex Runtime

- Implement the continuity handoff contract in `.planning/superpowers/specs/2026-04-16-codex-continuity-handoff-and-compaction-contract-design.md`: one fresh-thread handoff path for rollover, resume-failed recovery, usage exhaustion, and force-fresh.
- Add summary-backed handoff generation on top of the now-fixed raw-tail rule (`summary + last 5 complete raw turns`). Blocked on memory/handoff-summary implementation, not on payload-shape or tail-length decisions.

## Onboarding And Branding

- Expand `codex-claw onboard` into a guided setup that lets the operator choose one initial chat surface (`telegram` or `discord`) and writes only that channel's setup path by default.
- Add an onboarding import path for an existing `auth.json`, copying it into the managed live Codex home (`CODEX_CLAW_HOME/codex-home/auth.json`) instead of assuming the user starts from an already-authenticated shell.
- Define one canonical brand source and finish the rebrand pass across onboarding copy, workspace persona text, CLI/help/status surfaces, config examples, and docs.

## Compaction

- Implement the hybrid compaction signal contract from `.planning/superpowers/specs/2026-04-16-codex-continuity-handoff-and-compaction-contract-design.md`: prefer fresh live Codex context signal, fall back to local estimate when live signal is absent or stale.

## Voice

- If voice support returns later, reintroduce ASR/TTS through an explicit runtime-native config path.
