# Followups

## Codex Runtime

- Add handoff behavior for rollover threads (`summary + last few raw turns`) once the memory strategy is settled.
- Replace the current transcript-derived fresh-thread bootstrap with a better handoff-summary policy once the memory strategy is settled.

## Compaction

- Decide whether proactive compaction should eventually use live Codex-reported context signals instead of only local token estimates.

## Voice

- If voice support returns later, reintroduce ASR/TTS through an explicit runtime-native config path.
