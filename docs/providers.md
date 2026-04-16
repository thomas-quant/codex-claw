# Runtime Models

## Supported Runtime Paths

The active runtime surface has two paths:

- `codex`: primary runtime via `codex app-server`
- `deepseek`: fallback HTTP path

Everything else is out of scope for the active operator docs, even if some shared HTTP/OpenAI-compatible plumbing still exists under the hood.

Legacy fallback arrays such as `model_fallbacks`, `image_model_fallbacks`, `model.fallbacks`, and `subagents.model.fallbacks` are deprecated. The app may still parse them for compatibility, but the runtime ignores them.

## Codex

Codex is the default runtime. Bare model ids are treated as Codex models, so agent config can stay simple:

```yaml
model: gpt-5.4-mini
```

Runtime defaults live in `runtime.codex`:

```json
{
  "runtime": {
    "codex": {
      "default_model": "gpt-5.4",
      "default_thinking": "medium",
      "fast": false
    }
  }
}
```

Codex authentication is external. The app starts `codex app-server` inside an already-authenticated shell and does not read Codex credentials from config.

### Dedicated Codex Home And Account Switching

The Codex provider always launches `codex app-server` with:

```text
CODEX_HOME=<CODEX_CLAW_HOME>/codex-home
```

That dedicated live home is separate from the saved account snapshots under `CODEX_CLAW_HOME/codex-accounts/accounts/`.

When multiple Codex accounts are configured, the coordinator wraps the app-server runtime:

- it refreshes fresh Codex account telemetry before making a soft-switch decision, including the `account/read` and `account/rateLimits/read` runtime surface, with rate-limit headroom driving the actual switch choice
- it refreshes Codex account telemetry before making a soft-switch decision, including the `account/read` and `account/rateLimits/read` runtime surface, with rate-limit headroom driving the actual switch choice
- it only switches between turns; auth is not swapped in the middle of a running turn
- after a swap, it tries same-thread resume first so the current conversation can continue on the new account
- if same-thread recovery cannot continue safely, the fresh-thread fallback seeds the next request from the last five raw turns

Usage exhaustion can still force a hard switch after a failed turn. That path follows the same rule: swap at the turn boundary, try same-thread recovery first, then fall back to a fresh thread when needed.

## DeepSeek Fallback

DeepSeek is configured only through the runtime fallback block:

```json
{
  "runtime": {
    "fallback": {
      "deepseek": {
        "enabled": true,
        "model": "deepseek-chat",
        "api_base": "https://api.deepseek.com/v1"
      }
    }
  }
}
```

The runtime reads `DEEPSEEK_API_KEY` from the environment when building the fallback provider. Keep that secret out of `config.json`.

## Agent Model Overrides

- `agents.defaults.model_name` can still seed startup behavior during the transition
- AGENT/frontmatter `model` is the preferred per-agent override
- per-thread commands such as `/set model`, `/set thinking`, and `/fast` override both until changed again

Those overrides do not create a configurable fallback chain. Automatic fallback remains runtime-owned, with Codex primary and the optional DeepSeek path above.

## Voice Note

Voice support is not part of the active Codex-first runtime contract. If it returns, it should use an explicit runtime-native config path instead of the deleted provider catalog.
