# Security Configuration

This fork keeps `.security.yml` for non-model secrets only. Use it for channel tokens and tool credentials that should not live in `config.json`.

## What Belongs Here

- `channels.telegram.token`
- `channels.discord.token`
- tool credentials under `tools.web`, `tools.skills`, and similar `SecureString` fields

Codex auth is not configured here. The Codex runtime starts inside an auth-prepared environment managed by the external auth package. DeepSeek fallback credentials also stay out of `config.json`; set `DEEPSEEK_API_KEY` in the process environment.

## File Layout

```text
~/.picoclaw/
├── config.json
└── .security.yml
```

`.security.yml` overrides matching secret fields from `config.json`. You can omit those secret fields from the main config entirely.

## Example

```yaml
channels:
  telegram:
    token: "1234567890:telegram-token"
  discord:
    token: "discord-bot-token"

web:
  brave:
    api_keys:
      - "BSA..."
  tavily:
    api_keys:
      - "tvly-..."

skills:
  github:
    token: "ghp_..."
```

## Rules

- Keep `.security.yml` out of version control.
- Set permissions tightly, for example `chmod 600 ~/.picoclaw/.security.yml`.
- Legacy `model_list` and `providers` keys are rejected when loading `.security.yml`.
- Secret mapping is structural. There are no `ref:` indirections to maintain.

## Verification

Start the CLI or gateway after updating secrets:

```bash
picoclaw gateway
```

If a required token is missing, the affected channel or tool will fail during startup rather than silently falling back.
