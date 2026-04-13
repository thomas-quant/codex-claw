# Runtime Models

## Supported Runtime Paths

This fork keeps two model paths:

- `codex`: primary runtime via `codex app-server`
- `deepseek`: fallback HTTP path

Everything else from the old provider matrix is removed.

## Codex

Codex is the default runtime. Bare model ids are treated as Codex models in the forked runtime, so agent config can stay simple:

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

Codex authentication is external. The app starts `codex app-server` inside an auth-prepared environment and does not read Codex credentials from config.

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

## Voice Note

Legacy voice auto-detection through `model_list` is gone. If voice support stays in the fork, it should come back as an explicit runtime-native config path rather than through the deleted provider catalog.
