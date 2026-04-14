# Codex Account Balancing V1 Design

Date: 2026-04-14

## Summary

Build Codex account balancing directly into `codex-claw` using a dedicated Codex home, one globally active account at a time, fresh Codex app-server telemetry, and between-turn account switching.

The v1 design intentionally optimizes for correctness and debuggability over maximum concurrency:

- one dedicated Codex home for `codex-claw`
- one active account in that home at a time
- one Codex app-server process for that home at a time
- one global single-flight guard for all Codex turns and auth mutation
- automatic switching only between turns, never during a healthy turn

Bindings stay unchanged because Codex threads remain resumable after `auth.json` swaps inside the same Codex home.

## Goals

- Use multiple Codex accounts as one pooled capacity source for one operator.
- Keep existing Codex thread continuity whenever possible.
- Switch accounts automatically when short-window usage is nearly exhausted.
- Switch accounts automatically after explicit usage exhaustion.
- Keep auth snapshots per account and sync refreshed auth back safely.
- Allow adding accounts while `codex-claw` is running through isolated add flow.
- Keep runtime state isolated from the user's default Codex home.

## Non-Goals

- No multi-user scheduling.
- No true parallel multi-account runtime in v1.
- No per-account thread sharding in v1.
- No round-robin balancing in v1.
- No weighted static routing tables in v1.
- No separate binding model per account.
- No `/tmp`-based isolated account homes.
- No mid-turn switch for healthy turns.

## User Model

One operator has multiple Codex accounts. `codex-claw` should treat them like interchangeable credentials for the same runtime capability.

The operator wants:

- one dedicated Codex runtime used only by `codex-claw`
- account snapshots stored under operator-controlled app storage
- automatic switching near exhaustion
- exact-thread resume after switch when possible
- fallback to fresh thread with recent history when resume still fails
- ability to add accounts headlessly and while runtime is active

## Chosen Approach

Three approaches were considered:

1. Minimal integrated switcher inside `codex-claw`
2. Separate daemon-side scheduler
3. Per-account isolated runtime homes with parallel app-servers

Option 1 is chosen for v1.

Why:

- matches already-validated behavior: same thread can resume after auth swap inside same Codex home
- smallest unknown-unknown set
- easiest to debug from logs and files
- requires no new long-lived sidecar process
- keeps later upgrade path open toward richer daemon scheduling

## Baseline Repo Constraints

Current runtime already has:

- Codex-first provider path via `codex app-server`
- persistent per-thread bindings
- per-thread runtime controls
- narrow fallback behavior

Current runtime does not have:

- real Codex account pool
- real Codex backend load balancing
- per-account health-aware routing

V1 should extend current Codex runtime rather than replace it.

## Architecture

### Dedicated Codex Home

`codex-claw` uses a dedicated Codex home isolated from the user's default Codex home.

Suggested layout under app-owned storage:

- `<CODEX_CLAW_HOME>/codex-home/`
- `<CODEX_CLAW_HOME>/codex-accounts/state.json`
- `<CODEX_CLAW_HOME>/codex-accounts/accounts/<alias>.json`
- `<CODEX_CLAW_HOME>/codex-accounts/health.json`
- `<CODEX_CLAW_HOME>/codex-accounts/switches.jsonl`
- `<CODEX_CLAW_HOME>/isolated-homes/<id>/`

This design uses application-owned storage, not `/tmp`, for isolated add flows.

### Active Runtime Model

At runtime:

- one account is active in dedicated live `auth.json`
- one Codex app-server process serves that dedicated Codex home
- all Codex turns run behind one global single-flight guard
- all auth mutation runs behind same guard

This means concurrent Codex-backed turns are serialized in v1. That is acceptable because correctness matters more than throughput for first version.

### Bindings And Threads

Bindings remain unchanged.

Reason:

- thread location is determined by Codex home
- account switch swaps auth only
- validated behavior shows same `thread_id` can resume after auth swap in same Codex home

Therefore no `binding -> account` schema change is required in v1.

## Account Snapshot Model

Each account alias stores one opaque Codex `auth.json` snapshot.

The alias is an operator-facing label only. It is not a semantic routing shard. Accounts are treated as interchangeable credentials with different health states.

Snapshot rules:

- activate account: copy alias snapshot into live `auth.json`
- switch away: copy live `auth.json` back into current alias snapshot
- once target snapshot is copied successfully into live `auth.json`, persisted active-alias state must update to target immediately
- parse `last_refresh` inside `auth.json`
- if `last_refresh` age is less than 6 hours, boundary sync is enough
- if `last_refresh` age is 6 hours or more, sync live auth back after each completed turn too

This reduces crash loss of refreshed tokens for older sessions without forcing constant writes for fresh sessions.

## Telemetry Model

### Authority

Soft-switch decisions must use fresh Codex app-server RPC telemetry queried at decision time.

Persisted health is cache and observability only. It is not authority for soft switching.

### Metrics

Track per account:

- `5h_remaining_pct`
- `weekly_remaining_pct`
- `5h_reset_at`
- `weekly_reset_at`
- `observed_at`

### Freshness Policy

Decision-time policy:

- before soft switch, query fresh telemetry now
- if fresh query succeeds, use it
- if fresh query fails, do not soft switch
- hard exhaustion may still trigger account switch even when fresh pre-switch telemetry is unavailable

This is intentionally conservative.

## Routing And Health Policy

### Health States

Derived states:

- `healthy`
- `soft-drain`
- `weekly-drain`
- `exhausted`
- `unknown`

Interpretation:

- `healthy`: usable, not near threshold
- `soft-drain`: active account at or below soft threshold, should switch before next turn
- `weekly-drain`: avoid using if alternatives exist because weekly budget is below floor
- `exhausted`: explicit usage exhaustion or effectively zero usable headroom
- `unknown`: no trustworthy fresh telemetry

### Soft Trigger

Soft switching happens only between turns.

Rule:

- if active account `5h_remaining_pct <= 10`, arm switch before next turn

No healthy turn is interrupted mid-flight.

### Hard Trigger

Hard switching happens after explicit Codex usage exhaustion has already stopped the turn.

### Target Selection

Primary ranking:

1. prefer accounts with `weekly_remaining_pct >= 20`
2. among those, maximize `5h_remaining_pct`
3. tie-break with `weekly_remaining_pct`

Fallback behavior:

- if no healthy target exists, try least-bad accounts anyway
- if all targets fail, warn and stop

### Non-Goals For V1 Routing

V1 explicitly does not use:

- round robin
- static weighted routing tables
- per-thread sharding

Those add complexity without solving first-order correctness problems.

### Hysteresis

V1 does not add a separate hysteresis band yet.

Reason:

- switching already happens only between turns
- operator scope is single-user
- threshold policy is simple enough for first version

If flapping appears in practice, add drain cooldown or exit/re-entry thresholds in v2.

## Switch Workflow

### Between-Turn Soft Switch

When current turn ends and soft threshold is crossed:

1. acquire global Codex mutation guard
2. query fresh telemetry
3. select target account
4. stop Codex app-server
5. sync live auth back to source snapshot if required by sync policy
6. copy target snapshot into live `auth.json`
7. start Codex app-server
8. resume same thread
9. release guard

### Hard Exhaustion Switch

When Codex stops turn with usage exhaustion:

1. acquire global Codex mutation guard
2. select target account, preferring healthy, then least-bad
3. stop Codex app-server
4. sync live auth back to source snapshot if required
5. swap target snapshot into live `auth.json`
6. start Codex app-server
7. resume same thread
8. if resume still fails after bounded retry, start fresh thread seeded from last 5 raw turns
9. release guard

## Failure Handling

### Auth Swap Failure

If copying target snapshot into live `auth.json` fails:

- mark that candidate switch attempt failed
- try different candidate account

### App-Server Start Failure

If app-server fails to start after auth swap:

- do not treat failure as account-health signal
- do not penalize account health
- do not continue rotating through accounts automatically
- keep persisted active-alias state aligned with live `auth.json`, which already points at target account
- surface runtime startup failure

Rationale: startup failure is not inherently account-related.

### Resume Failure

If same-thread resume fails on new account:

- retry bounded number of times
- if still failing, create fresh thread
- seed fresh thread with raw last 5 turns from session history JSONL

Bootstrap intentionally includes:

- last 5 user/assistant turns only

Bootstrap intentionally excludes:

- full reconstructed session context
- synthetic tool summary
- extra state not already present in raw recent turns

This keeps degraded recovery deterministic and bounded.

### Telemetry Failure

If fresh telemetry query fails before soft switch:

- do not soft switch
- keep runtime on current account

If hard exhaustion occurs later:

- switch attempt is still allowed

## Add Account Flows

### Normal Add

If Codex runtime is not active:

- account can be added without isolation

### Isolated Add

If Codex runtime is active:

- account add must use `--isolated`
- isolated home must live under app-owned directory such as `<CODEX_CLAW_HOME>/isolated-homes/<id>/`
- isolated flow must not mutate dedicated live runtime home

### Device Auth

Headless account enrollment must support `--device-auth`.

`--device-auth` may be combined with `--isolated`.

### Isolation Rule

Isolation is required only when runtime is active. It is not required for every add.

## State And Logging

### Persisted State

Persist:

- active alias
- alias snapshot inventory
- latest observed telemetry cache
- switch audit trail

Keep state simple and file-based in v1.

### Logging Fields

Every switch-related log should include:

- `alias`
- `account_health`
- `route_reason`
- `switch_trigger`
- `5h_remaining_pct`
- `weekly_remaining_pct`
- `telemetry_fresh`
- `resume_mode`
- `app_server_restart`

Recommended `route_reason` values:

- `soft_threshold_5h`
- `hard_exhaustion`
- `weekly_drain_avoidance`
- `best_5h_headroom`
- `least_bad_fallback`
- `all_accounts_failed`

Recommended `resume_mode` values:

- `same_thread_resume`
- `fresh_thread_last5`

## User-Facing Operations

V1 must support:

- add account
- add account with `--device-auth`
- add account with `--isolated`
- remove account
- list accounts
- show active account
- show account health/status
- enable account
- disable account

Force-switch command is useful but not required for first implementation pass.

## Security And Safety

- dedicated Codex home must not reuse user's default Codex home
- account snapshots must live in private app-owned storage
- auth snapshot files must use restrictive permissions
- isolated homes must also use restrictive permissions
- mutation must fail closed if guard ownership is unclear
- runtime must never mutate auth while healthy turn is still running

## Testing Strategy

### Unit Tests

Cover:

- account ranking logic
- weekly-drain avoidance
- soft threshold rule
- telemetry freshness gating
- sync policy from `last_refresh`
- switch failure branching
- degraded recovery bootstrap from last 5 turns

### Integration Tests

Cover:

- same-thread resume after account swap in dedicated Codex home
- switch after explicit exhaustion
- soft switch before next turn
- auth sync-back on switch boundary
- per-turn sync when `last_refresh >= 6h`
- isolated add while runtime active
- device-auth add path
- startup failure after auth swap surfaces runtime error without health penalty

### Recovery Tests

Cover:

- crash during switch before auth swap
- crash after auth swap before app-server start
- resume failure leading to fresh-thread fallback
- telemetry query failure skipping soft switch

## Implementation Boundaries

V1 implementation should be split into focused units:

- account snapshot store
- live auth synchronizer
- Codex mutation guard
- telemetry reader
- health evaluator
- target selector
- switch coordinator
- degraded recovery bootstrapper
- account add workflow manager

This keeps account logic isolated from unrelated agent-loop concerns.

## Future Work

Possible v2 follow-ups:

- daemon-side monitor
- hysteresis bands
- force-switch command
- richer health dashboards
- per-account isolated runtimes
- parallel account pool

None of those are required for v1.
