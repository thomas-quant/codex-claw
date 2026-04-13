# Configuration

## Paths

- Default config: `~/.picoclaw/config.json`
- Default home: `~/.picoclaw`
- Default workspace: `~/.picoclaw/workspace`

Supported environment overrides:

- `PICOCLAW_CONFIG`: load a specific config file
- `PICOCLAW_HOME`: relocate the default home and workspace roots

## Runtime Block

The runtime is Codex-first. `runtime.codex` sets the default Codex model and turn behavior:

```json
{
  "runtime": {
    "codex": {
      "default_model": "gpt-5.4",
      "default_thinking": "medium",
      "fast": false,
      "auto_compact_threshold_percent": 30,
      "discovery_fallback_models": ["gpt-5.4", "gpt-5.4-mini"]
    }
  }
}
```

`runtime.fallback.deepseek` is the only built-in fallback path. It carries the model id and API base only. DeepSeek auth should come from your external secret or env setup, not from `config.json`.

Legacy fallback arrays are no longer part of the runtime contract. The following fields are still parseable for compatibility, but the runtime warns and ignores them:

- `agents.defaults.model_fallbacks`
- `agents.defaults.image_model_fallbacks`
- structured `model.fallbacks`
- structured `subagents.model.fallbacks`

Codex auth is not configured here at all. Start `codex app-server` in a shell where Codex is already authenticated.

## Agents

Keep using the existing agent backbone:

- `agents.defaults.workspace` sets the main workspace root
- AGENT/frontmatter `model` stays a raw Codex model id such as `gpt-5.4-mini`
- per-thread runtime commands can override model, thinking, and `fast` without changing static config

Do not rely on fallback arrays in agent config. Active fallback behavior is runtime-owned rather than configured through legacy `fallbacks` lists.

Example:

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace",
      "max_tokens": 8192,
      "max_tool_iterations": 20
    }
  }
}
```

## Channels

Telegram and Discord are the supported chat channels. The generic channel manager stays, but other channel configs are out of scope.

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_TELEGRAM_BOT_TOKEN",
      "allow_from": ["YOUR_USER_ID"]
    },
    "discord": {
      "enabled": false,
      "token": "YOUR_DISCORD_BOT_TOKEN"
    }
  }
}
```

## Workspace Layout

The workspace remains the operational center:

- `sessions/`: local chat history and mirrored runtime state
- `memory/`: long-term memory files
- `cron/`: scheduled jobs
- `skills/`: custom skills
- `AGENT.md`, `USER.md`, `SOUL.md`, `HEARTBEAT.md`: runtime behavior files

## Bindings and MCP

`bindings`, `tools.mcp`, hooks, cron, memory, and isolation stay first-class. The main change is the runtime surface, not the agent/tool backbone.

Use agent-level `mcpServers` allowlists when you want to grant MCP tools to a specific agent. This allowlist is strict: if `mcpServers` is omitted, that agent receives no MCP tools by default.
