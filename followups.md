# Followups

## Codex Runtime

- Add handoff behavior for rollover threads (`summary + last few raw turns`) once the memory strategy is settled.
- Replace the current transcript-derived fresh-thread bootstrap with a better handoff-summary policy once the memory strategy is settled.

## Compaction

- Decide whether proactive compaction should eventually use live Codex-reported context signals instead of only local token estimates.

## Runtime Cleanup

- Deduplicate the interactive and legacy tool-execution paths in `pkg/agent` so Codex and non-interactive providers do not maintain parallel tool result handling logic.
- Remove or hard-deprecate legacy `model_fallbacks` config/frontmatter fields now that automatic runtime failover is explicitly Codex -> DeepSeek only.
- Tighten recovery behavior further if needed; runtime now supports restart + resume retry, but future work may want more explicit recovery-state handling from the agent layer.
- If voice support returns later, reintroduce ASR/TTS through an explicit runtime-native config path.
- Rewrite the matching localized ASR/TTS docs if they are kept, or delete them if voice docs are not worth maintaining during the fork.
- Replace removed channel names used as generic examples in tests/docs where they no longer reflect the fork boundary (`whatsapp`, `slack`, `feishu`, etc.).
- Review localized docs for any remaining deleted-channel, deleted-migration, or provider-era setup references.
- Sweep remaining comments/help text for inaccurate legacy terminology such as `model_list`, `providers`, and OAuth-era auth wording where the runtime has already moved on.

## Fork Cleanup

- Remove any final stale docs or examples that still imply launcher/web surfaces, old migration support, or deleted channel families.
