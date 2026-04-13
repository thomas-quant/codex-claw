# Sensitive Data Filtering

Tool results can be scrubbed before they are sent back into Codex or the fallback model. This prevents obvious secret leaks when a tool echoes tokens, passwords, or API keys.

## Config

Filtering lives under `tools`:

```json
{
  "tools": {
    "filter_sensitive_data": true,
    "filter_min_length": 8
  }
}
```

- `filter_sensitive_data`: enable or disable replacement
- `filter_min_length`: skip very short strings for speed

## What Gets Filtered

Sensitive values are collected from the loaded config and `.security.yml`, then replaced with `[FILTERED]` before tool output is sent back to the model.

Typical examples:

- Telegram and Discord bot tokens
- web search API keys
- skill registry tokens
- other `SecureString` or `SecureStrings` values stored in channel and tool config

Codex auth is outside this config flow, so it is not part of the replacement set.

## Example

If `.security.yml` contains:

```yaml
channels:
  telegram:
    token: "123456:ABC-DEF"
```

and a tool returns:

```text
using token 123456:ABC-DEF
```

the model receives:

```text
using token [FILTERED]
```

## Notes

- Filtering is a guardrail, not a substitute for keeping secrets out of prompts.
- Replacement uses a cached `strings.Replacer`, so normal tool traffic stays cheap.
- Keep it enabled unless you are actively debugging raw tool output.
