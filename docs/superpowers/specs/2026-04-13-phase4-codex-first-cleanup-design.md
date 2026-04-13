# Phase 4: Codex-First Cleanup Design

## Goal

Replace PicoClaw's broad provider, auth, launcher, and channel surface with a Codex-first runtime shape that preserves the existing agent backbone. This phase is intentionally destructive: it is a hard schema break and a physical tree cleanup, not a compatibility refactor.

## Scope

This phase rewrites or deletes:

- provider/model/auth config
- old provider creation and selection code
- launcher and web surfaces
- old auth CLI and auth package
- non-kept channels
- non-kept providers
- docs, examples, tests, and build targets tied to removed surfaces

This phase preserves:

- multi-agent and workspace-agent model
- hooks, skills, cron, memory, MCP, isolation, session handling
- generic channel manager
- Telegram and Discord channels
- Codex app-server runtime path
- DeepSeek fallback path

## Recommended Approach

Use a constrained fork cleanup in one giant patch.

Alternatives considered:

1. Compatibility-first cleanup. Lower immediate breakage, but carries old config, migration, and provider ballast deeper into the fork.
2. Incremental deletion across many small passes. Easier to review, but leaves the tree in an incoherent half-migrated state for too long.
3. Single destructive cleanup pass. Recommended.

The single-pass cleanup matches the fork intent: this is no longer a general provider platform.

## Runtime and Config Target State

Keep the existing top-level operational surface where it still makes sense:

- `agents`
- `bindings`
- `session`
- `channels`
- `tools`
- `hooks`
- `heartbeat`
- `isolation`
- `devices`
- `voice`

Delete the old provider and auth surface:

- `model_list`
- provider selection in `agents.defaults`
- legacy model fallback fields tied to the removed provider stack
- OAuth and token-auth config branches
- old config compatibility and migration layers whose purpose is preserving deleted provider/auth structure

Add a new top-level `runtime` block:

- `runtime.codex.default_model`
- `runtime.codex.default_thinking`
- `runtime.codex.fast`
- `runtime.codex.auto_compact_threshold_percent`
- `runtime.codex.discovery_fallback_models`
- `runtime.fallback.deepseek.enabled`
- `runtime.fallback.deepseek.model`
- `runtime.fallback.deepseek.api_base`

Codex auth does not exist in config. It will be handled by a separate Codex auth package.

AGENT/frontmatter `model` remains valid and is a raw Codex model id such as `gpt-5.4-mini`.

Per-thread overrides remain runtime state, not static config:

- `/set model`
- `/set thinking`
- `/fast`

## Rewrite In Place

The cleanup patch should rewrite these surfaces rather than delete them:

- `cmd/picoclaw/main.go`
- `pkg/config/*`
- `pkg/providers/factory_provider.go`
- `pkg/providers/legacy_provider.go`
- `pkg/channels/manager.go`
- `pkg/channels/manager_channel.go`
- `pkg/commands/*`
- `Makefile`
- `go.mod`
- `go.sum`
- `config/config.example.json`

Expected changes:

- remove auth, launcher, and old provider command wiring from the root CLI
- collapse provider creation to Codex primary plus DeepSeek fallback
- keep the generic channel manager but register only Telegram and Discord
- keep the Codex runtime command surface and remove old provider-era command behavior
- simplify build and test targets to match the smaller product surface

## Physical Deletions

Delete these top-level surfaces entirely:

- `web/`
- `cmd/picoclaw-launcher-tui/`
- `cmd/picoclaw/internal/auth/`
- `pkg/auth/`

Delete non-kept providers under `pkg/providers/`, including direct Codex auth-backed providers and unrelated provider families. Keep only:

- Codex app-server provider path
- DeepSeek HTTP/OpenAI-compatible support
- shared provider utilities still required by the surviving runtime

Delete non-kept channels under `pkg/channels/`. Keep only:

- `telegram/`
- `discord/`
- shared channel framework files still used by the generic manager

Delete docs and tests tied only to removed auth, launcher, provider, and channel surfaces.

## Failure Order During the Giant Patch

The patch will temporarily break in a predictable order:

1. imports fail after auth, launcher, provider, and channel files are deleted
2. config compile errors appear after provider/auth schema is removed
3. command and test failures appear once old model-switch and auth behavior is gone
4. dependency pruning and docs/examples cleanup finish the pass

This order is acceptable. The patch should be reviewed as one coherent cleanup rather than a sequence of green intermediate states.

## Build and Docs Cleanup

`Makefile` should drop:

- launcher build targets
- web build targets
- WhatsApp-native build targets
- any build branches that only existed for deleted surfaces

Docs should be reduced to the fork-relevant surface:

- runtime config
- Telegram and Discord
- MCP
- agents, hooks, skills
- cron
- isolation

Old launcher, OAuth, and removed-provider docs should be deleted rather than left stale.

## Testing Strategy

Keep and repair tests for:

- Codex runtime
- DeepSeek fallback
- agent loop
- MCP
- hooks
- cron
- Telegram and Discord
- config parsing for the new hard-broken schema

Delete tests that only validate:

- removed providers
- removed channels
- removed auth flows
- launcher behavior
- old config migration behavior

Verification for the cleanup phase should focus on:

- `go test ./pkg/config ./pkg/providers ./pkg/channels ./pkg/commands ./pkg/agent`
- broader repo test runs only after the deletion map compiles again

## Non-Goals

This phase does not implement:

- the future Codex auth package
- rename/rebrand work
- media-input expansion
- new memory semantics beyond existing backbone behavior
- transport changes beyond the already-selected Codex app-server `stdio` path

## Risks

- stale imports and transitive references from docs/tests/build scripts
- hidden config migration dependencies
- surprising shared utility dependencies inside `pkg/providers` and `pkg/channels`
- overly broad deletion of files that appear unused but still anchor tests or registration

The mitigation is to review the cleanup as a deliberate hard break, then repair compile and test fallout in the same patch before moving to the next phase.
