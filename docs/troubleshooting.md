# Troubleshooting

## Codex app-server will not start

Symptoms:

- startup fails when a Codex-backed agent runs
- interactive turns immediately error before streaming begins

Checks:

1. Confirm the Codex binary and app-server entrypoint are installed and runnable in the current shell.
2. Confirm you launched PicoClaw from a shell where Codex is already authenticated.
3. Check the process logs for the first startup error instead of retrying blindly.

## No default model is available

This fork no longer uses the old model catalog. Runtime defaults live under `runtime.codex`.

Check:

- `runtime.codex.default_model`
- optional per-agent frontmatter `model`

If a thread-specific override was set earlier, `/status` will show the effective model and the current thread continuity markers:

- last user message time
- last compaction time
- recovery state

## DeepSeek fallback never activates

DeepSeek is a narrow fallback. It is only used when Codex cannot start, connect, resume, or hits usage exhaustion.

Checks:

- `runtime.fallback.deepseek.enabled` is `true`
- `runtime.fallback.deepseek.model` is set
- `DEEPSEEK_API_KEY` is present in the environment

If a live Codex turn fails for some other reason, fallback is intentionally not automatic.

If you configured any of these legacy fields, they will not activate fallback behavior:

- `agents.defaults.model_fallbacks`
- `agents.defaults.image_model_fallbacks`
- structured `model.fallbacks`
- structured `subagents.model.fallbacks`

Current builds keep those fields parseable for compatibility, but they log a deprecation warning and are ignored.

## Telegram or Discord will not connect

Checks:

- the channel is enabled in `config.json`
- the matching token exists in `.security.yml`
- `allow_from` is correct for the users you expect to talk to the bot

Use `picoclaw gateway` to surface channel startup errors directly.

## MCP tools are missing

Checks:

- the relevant MCP server is configured under `tools.mcp`
- `tools.mcp.enabled` is `true`
- the active agent explicitly lists that server in `mcpServers`
- the MCP server command or endpoint is reachable from the current runtime environment
- if `mcpServers` is omitted, the agent receives no MCP tools by default

This fork keeps PicoClaw-managed MCP. Codex-native MCP is intentionally disabled.
