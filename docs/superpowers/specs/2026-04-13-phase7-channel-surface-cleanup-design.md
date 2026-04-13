# Phase 7: Channel Surface Cleanup Design

## Goal

Finish the channel cleanup by making the surviving fork surface consistently present only Telegram and Discord. Phase 7 is not another channel-runtime refactor. It is a debris sweep across English docs, examples, tests, and residual operator-facing text so the fork stops implying support for removed integrations.

## Scope

This phase adds:

- cleanup of English docs and examples that still advertise removed channels
- cleanup of tests that use removed channel names as generic fixtures where that now misrepresents the fork
- cleanup of residual channel-facing text in code, help output, status text, and examples

This phase does not add:

- changes to Telegram or Discord behavior
- another `pkg/channels` architecture refactor
- localized docs cleanup
- gateway/runtime changes beyond removing stale removed-channel references

## Recommended Approach

Use a narrow debris sweep.

Alternatives considered:

1. Deeper channel simplification. This would try to collapse the shared channel framework around only two channels, but that is unnecessary churn because the generic manager and base interfaces are still useful.
2. Docs-only cleanup. Too narrow; it leaves tests and user-facing text inconsistent with the fork.
3. Narrow debris sweep. Recommended.

Under the recommended approach:

- keep `pkg/channels` shared framework intact
- keep gateway/channel runtime wiring intact unless a stale removed-channel reference is still compiled
- focus on what users and contributors actually see: docs, examples, tests, and top-level text

## Docs And Example Policy

English-facing docs should consistently describe:

- Telegram and Discord as the only shipped chat channels
- the shared channel manager as the runtime host for those two channels
- `.security.yml` and `config.json` examples using only Telegram and Discord channel tokens and settings

Docs should stop implying support for Matrix, WhatsApp, Slack, QQ, WeCom, Feishu, IRC, email-style channel workflows, or other removed upstream integrations unless the reference is explicitly historical and still useful.

Configuration snippets should only include:

- `channels.telegram`
- `channels.discord`

If a doc is now mostly about removed channels, delete it instead of leaving a confusing stub.

## Test And Fixture Policy

Tests may still use synthetic channel names where the behavior is deliberately generic, but Phase 7 should remove examples that misleadingly present removed channels as part of the supported product surface.

The practical rule is:

- keep generic tests generic when channel identity does not matter
- rewrite tests using removed real channel names as examples when those names appear in user-facing assertions, snapshots, or help text
- preserve coverage of generic manager behavior without pretending removed channels still exist

## Code Text Policy

Audit residual operator-facing text in:

- CLI/help output
- config/help comments
- status/debug text
- top-level docs referenced by commands

Remove or rewrite wording that still suggests removed channels are available in this fork.

This does not require changing internal interfaces just because they use the word `channel`. It only requires cleaning user-visible references to removed concrete integrations.

## Likely Touch Points

Expected files or areas:

- `docs/chat-apps.md`
- `docs/channels/discord/README.md`
- `docs/channels/telegram/README.md`
- `docs/configuration.md`
- `docs/security_configuration.md`
- `docs/troubleshooting.md`
- `config/config.example.json`
- `pkg/channels/README.md`
- `pkg/channels/manager_runtime_test.go`
- any tests or help text still naming removed channels in a product-facing way

The exact file list should stay small and evidence-driven.

## Testing Strategy

Keep verification narrow and consistency-focused.

Required checks:

- targeted tests for any touched channel-facing test files still pass
- `go test ./pkg/channels ./cmd/picoclaw -count=1` if channel-facing code/help text changes
- manual diff review confirming that English docs/examples no longer advertise removed channels

This phase is primarily about correctness of the presented product surface, not new runtime behavior.

## Risks

- over-cleaning generic tests that are not actually product-facing
- accidentally removing still-relevant shared channel framework docs
- spending time on broad doc rewrites instead of deleting dead references directly

The mitigation is to keep the phase narrow: clean only what still misrepresents the fork, and leave the shared runtime architecture alone.
