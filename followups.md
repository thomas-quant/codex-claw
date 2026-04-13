# Followups

## Codex Runtime

- Implement real fresh-thread bootstrap from local session history instead of only forwarding the last user message.
- When resume ultimately fails, seed the fresh Codex thread from the last 3 local turns on disk instead of starting effectively empty.
- Implement the 8-hour inactivity rollover policy for Codex threads.
- Add handoff behavior for rollover threads (`summary + last few raw turns`) once the memory strategy is settled.

## Compaction

- Add proactive native Codex compaction for low-context situations; current native compaction is wired for manual `/compact` and interactive context-overflow retry, but not the full proactive threshold policy yet.
- Finish the agent/session compaction policy so Codex-native compaction cleanly replaces the generic context-manager path where appropriate, without affecting non-Codex providers.

## Runtime Cleanup

- Deduplicate the interactive and legacy tool-execution paths in `pkg/agent` so Codex and non-interactive providers do not maintain parallel tool result handling logic.
- Tighten recovery behavior further if needed; runtime now supports restart + resume retry, but future work may want more explicit recovery-state handling from the agent layer.

## Fork Cleanup

- Rewrite the config surface around Codex primary + DeepSeek fallback only.
- Remove old provider/model/auth machinery that the fork no longer needs.
- Remove unused channels and keep only the retained channel set.
- Remove `web/`, launcher, and related UI/auth surfaces that are no longer part of the fork.
