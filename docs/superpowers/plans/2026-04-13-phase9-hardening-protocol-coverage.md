# Phase 9: Hardening And Protocol Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Codex-first runtime by locking protocol-failure handling, recovery/fallback edge cases, and runtime-status invariants with focused tests and only the smallest required fixes.

**Architecture:** Keep the current Codex-first runtime shape. Phase 9 is a confidence pass, not a redesign. `pkg/codexruntime` remains the protocol/runtime layer, `pkg/providers` remains the adapter layer, and `pkg/agent` remains the orchestration layer.

**Tech Stack:** Go, `go test`

---

## File Structure

### Likely Modify

- `pkg/codexruntime/client_test.go`
- `pkg/codexruntime/runner_test.go`
- `pkg/codexruntime/status_test.go`
- `pkg/providers/codex_app_server_provider_test.go`
- `pkg/providers/error_classifier_test.go`
- `pkg/agent/loop_test.go`
- implementation files only if a contract bug is exposed by the new tests

### Keep As-Is

- config schema
- channel surface
- launcher/web surface

## Task 1: Harden Codex Protocol And Transport Failure Coverage

**Files:**
- Modify: `pkg/codexruntime/client_test.go`
- Modify: `pkg/codexruntime/status_test.go` if needed
- Modify: `pkg/codexruntime/client.go` only if tests expose contract bugs

- [ ] Add focused tests for malformed notification/server-request behavior that is still weakly covered.
- [ ] Add coverage for app-server exit/transport failure projection where the current client state or returned error is ambiguous.
- [ ] Keep assertions contract-based: final assistant projection, failure surfaced cleanly, no invalid hidden success path.
- [ ] Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/codexruntime -run 'TestClient_|TestBuildRuntimeStatus' -count=1
```

## Task 2: Harden Recovery, Continuity, And Runtime Status

**Files:**
- Modify: `pkg/codexruntime/runner_test.go`
- Modify: `pkg/providers/codex_app_server_provider_test.go`
- Modify: `pkg/agent/loop_test.go`
- Modify: implementation only if tests expose a real mismatch

- [ ] Add or tighten tests for restart+resume failure ordering, force-fresh transitions, compaction metadata, and persisted status projection.
- [ ] Add loop-level assertions that runtime status/controls remain coherent after reset, compaction, and fallback-triggering failures.
- [ ] Ensure tests cover the narrow fallback contract from Phase 8.
- [ ] Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/codexruntime ./pkg/providers ./pkg/agent -run 'Test(Runner_|CodexAppServerProvider_|AgentLoop_)' -count=1
```

## Task 3: Close With Package-Level Hardening Verification

**Files:**
- Modify any touched test/implementation files from Tasks 1-2

- [ ] Run the phase close-out sweep:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/codexruntime ./pkg/providers ./pkg/agent -count=1
```

- [ ] Review any failures for genuine contract bugs versus unrelated pre-existing noise. Fix only the former.
- [ ] If new implementation fixes were needed, add a short note to `followups.md` only for debt explicitly left behind.

## Notes

- Do not reopen generic provider-matrix behavior.
- Do not expand DeepSeek fallback beyond the explicit fork contract.
- Prefer a few strong tests over a large amount of low-signal mock coverage.
