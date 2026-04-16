# Troubleshooting

## Codex app-server will not start

Symptoms:

- startup fails when a Codex-backed agent runs
- interactive turns immediately error before streaming begins

Checks:

1. Confirm the Codex binary and app-server entrypoint are installed and runnable in the current shell.
2. Confirm you launched codex-claw from a shell where Codex is already authenticated.
3. Check the process logs for the first startup error instead of retrying blindly.

If you are using account balancing, also check that `CODEX_CLAW_HOME/codex-home/auth.json` exists and contains the live auth snapshot the app-server should read.

## `account add` fails while runtime is active

Symptom:

- `codex-claw account add <alias>` fails with an isolation-required error while Codex runtime work is already active

Why:

- the runtime keeps `CODEX_CLAW_HOME/codex-home/auth.json` live
- a non-isolated login would overwrite that live auth file mid-session

Fix:

1. Re-run the command with `codex-claw account add <alias> --isolated`.
2. Or stop the active Codex runtime first, then run a non-isolated add.
3. Keep the saved snapshot under `codex-accounts/accounts/<alias>.json`; do not edit `codex-home/auth.json` by hand unless you mean to replace the live account.

The isolated flow creates a temporary home under `CODEX_CLAW_HOME/isolated-homes/`, runs `codex login` there, copies back the resulting snapshot, then removes the temporary home.

## Existing Codex auth was not imported

Symptoms:

- onboarding skipped the auth import prompt even though you expected an existing login
- `codex-claw account import <alias> --auth-file <path>` fails
- `CODEX_CLAW_HOME/codex-home/auth.json` is still missing after import

Checks:

1. Confirm the source file really exists and is a Codex `auth.json`.
2. If you rely on auto-detection during onboarding, confirm `CODEX_HOME` points at the Codex home you expect. Without `CODEX_HOME`, onboarding looks for `~/.codex/auth.json`.
3. For an explicit import, re-run `codex-claw account import <alias> --auth-file <path-to-auth.json>`.
4. If you only need the live managed home and not a named snapshot yet, rerun `codex-claw onboard --import-auth-file <path-to-auth.json>`.

## No active account is shown

Symptoms:

- `codex-claw account status` reports configured accounts but `Active account: none`
- `codex-claw status` shows `Codex accounts` configured but `Active account` as `not set`

Checks:

1. Run `codex-claw account list` to confirm at least one account is enabled.
2. If every account is disabled, re-enable one with `codex-claw account enable <alias>`.
3. Confirm the live auth file exists at `CODEX_CLAW_HOME/codex-home/auth.json`.
4. If you have snapshots under `codex-accounts/accounts/` but no usable live auth, add or re-add the intended account, then restart the runtime so it can seed the live home again.

The account pool can store snapshots without an active live binding. Configured does not automatically mean active.

## Account switch succeeded but app-server restart failed

Symptoms:

- the switch log or status shows a new active alias
- the next turn still fails because the app-server could not resume or restart cleanly

Checks:

1. Inspect `CODEX_CLAW_HOME/codex-accounts/switches.jsonl` for the most recent `target_alias` and `trigger`.
2. Confirm `CODEX_CLAW_HOME/codex-home/auth.json` now matches the intended live account snapshot.
3. Retry the turn once. The runtime attempts same-thread resume first, then can fall back to a fresh thread seeded from the last five raw turns.
4. If the restart still fails, restart `codex-claw` or the affected worker process so a new `codex app-server` starts against the updated live home.
5. If the same account keeps failing, disable it with `codex-claw account disable <alias>` and leave another enabled account available for the next switch.

## No default model is available

The app no longer uses the old model catalog. Runtime defaults live under `runtime.codex`.

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

Use `codex-claw gateway` to surface channel startup errors directly.

## MCP tools are missing

Checks:

- the relevant MCP server is configured under `tools.mcp`
- `tools.mcp.enabled` is `true`
- the active agent explicitly lists that server in `mcpServers`
- the MCP server command or endpoint is reachable from the current runtime environment
- if `mcpServers` is omitted, the agent receives no MCP tools by default

The app keeps codex-claw-managed MCP. Codex-native MCP is intentionally disabled.
