# Phase 9: Hardening And Protocol Coverage Design

## Goal

Finish the Codex-first runtime pass by hardening the fragile edges instead of adding new product scope. Phase 9 is about confidence: protocol-failure handling, recovery/fallback edge cases, and status/command invariants.

## Scope

This phase adds:

- stronger Codex app-server protocol failure coverage
- tighter recovery and fallback policy verification
- runtime-status and command-path hardening
- focused tests for malformed frames, restart/resume edges, and fallback boundaries

This phase does not add:

- new channels
- config schema changes
- web/launcher work
- broader refactors unrelated to Codex runtime correctness

## Recommended Approach

Use a narrow hardening pass centered on tests plus the smallest implementation fixes required.

Alternatives considered:

1. Broad runtime refactor. Too risky and unnecessary after phases 1-8.
2. Tests only. Too optimistic; Phase 8 already showed that behavior drift can hide behind passing narrow tests.
3. Hardening pass with focused fixes. Recommended.

Under the recommended approach:

- treat the current runtime design as stable
- identify places where error handling is too permissive, ambiguous, or weakly tested
- fix only the behavior needed to satisfy the explicit Codex-first contract

## Primary Hardening Targets

### Codex protocol edges

Cover failure cases in `pkg/codexruntime` for:

- malformed or partial app-server notifications
- empty/invalid IDs on server requests
- stream failure ordering around `turn/start`
- app-server exit and transport error projection

### Recovery and continuity

Cover and tighten:

- resume failure after restart
- force-fresh thread behavior
- continuity metadata persistence
- status projection after recovery and compaction

### Fallback boundaries

The fork contract is now:

- automatic fallback only for Codex start/connect/resume failures and usage exhaustion
- no automatic fallback for ordinary live-turn failures

Phase 9 should ensure that behavior is locked by tests and not silently regressed by future loop changes.

### Runtime status and commands

Validate that:

- `/status`-backing runtime reads remain consistent after reset, compaction, fallback, and force-fresh
- thread control commands do not corrupt persisted runtime state
- per-thread settings survive where they should and reset only where intended

## Likely Touch Points

Expected files or areas:

- `pkg/codexruntime/client_test.go`
- `pkg/codexruntime/runner_test.go`
- `pkg/codexruntime/status_test.go`
- `pkg/providers/codex_app_server_provider_test.go`
- `pkg/providers/error_classifier_test.go`
- `pkg/agent/loop_test.go`

Implementation files should only change if tests expose real contract bugs.

## Testing Strategy

Keep verification focused on the Codex runtime boundary:

- `go test ./pkg/codexruntime -count=1`
- `go test ./pkg/providers -count=1`
- `go test ./pkg/agent -count=1`

If a narrower targeted loop is enough during iteration, that is acceptable, but the phase should close with the three-package sweep above.

## Risks

- overfitting tests to current implementation details instead of runtime contract
- re-opening generic provider logic that this fork is intentionally leaving behind
- accidentally broadening fallback again while hardening edge cases

Mitigation:

- keep the assertions centered on the documented fork behavior
- prefer black-box loop/provider/runtime tests over implementation-specific mocks where practical
- avoid config or channel surface churn during this phase
