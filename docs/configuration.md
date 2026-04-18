# Configuration

## Paths

- Default config: `~/.codex-claw/config.json`
- Default home: `~/.codex-claw`
- Default workspace: `~/.codex-claw/workspace`

Supported environment overrides:

- `CODEX_CLAW_CONFIG`: load a specific config file
- `CODEX_CLAW_HOME`: relocate the default home and workspace roots

`CODEX_CLAW_HOME` also becomes the dedicated Codex runtime-state root. The Codex provider keeps its live app-server auth and account-pool files there instead of reading auth from `config.json`.

`codex-claw onboard` is the guided first-run path. It asks you to pick one initial chat surface (`telegram` or `discord`), enables only that channel by default, and fills `allow_from` only for the chosen surface.

## Codex Home Layout

When account balancing is enabled, these paths under `CODEX_CLAW_HOME` matter:

- `codex-home/auth.json`: the live auth file mounted into `codex app-server`
- `codex-accounts/accounts/<alias>.json`: saved auth snapshot for one named account
- `codex-accounts/state.json`: enabled/disabled state plus the active alias
- `codex-accounts/health.json`: cached telemetry for account health and rate-limit headroom
- `codex-accounts/switches.jsonl`: append-only switch audit log
- `isolated-homes/<id>/`: temporary isolated homes used by `codex-claw account add --isolated`

The live file under `codex-home/` can change while the process runs. The snapshot files under `codex-accounts/accounts/` are the durable account copies used for later switches.

## Security Overlay

`.security.yml` is the separate secret overlay. Keep channel tokens and tool credentials there, not in `config.json`.

```yaml
channels:
  telegram:
    token: "YOUR_BOT_TOKEN"
web:
  brave:
    api_keys:
      - "YOUR_BRAVE_KEY"
```

See [security_configuration.md](security_configuration.md) for the matching secret layout and rules.

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
      "discovery_fallback_models": ["gpt-5.4", "gpt-5.4-mini"],
      "sandbox_mode": "workspace-write",
      "workspace_write": {
        "writable_roots": ["/workspace", "/tmp"],
        "network_access": true
      }
    }
  }
}
```

`runtime.codex.sandbox_mode` and `runtime.codex.workspace_write.*` are forwarded to Codex app-server turn requests. Fast-mode alignment is tracked separately and is not part of this runtime sandbox surface.

`runtime.fallback.deepseek` is the only built-in fallback path. It carries the model id and API base only. DeepSeek auth should come from your external secret or env setup, not from `config.json`.

Legacy fallback arrays are no longer part of the runtime contract. The following fields are still parseable for compatibility, but the runtime warns and ignores them:

- `agents.defaults.model_fallbacks`
- `agents.defaults.image_model_fallbacks`
- structured `model.fallbacks`
- structured `subagents.model.fallbacks`

Codex auth is not configured here at all. Start `codex app-server` in a shell where Codex is already authenticated.

## Account Commands

Use the built-in account commands to manage Codex auth snapshots under `CODEX_CLAW_HOME`:

- `codex-claw account add <alias>`: capture the current or newly-completed Codex login as a named account
- `codex-claw account add <alias> --isolated`: run `codex login` in a temporary isolated home, then save only the resulting snapshot
- `codex-claw account add <alias> --device-auth`: use device authorization instead of browser login
- `codex-claw account import <alias> --auth-file <path>`: import an existing Codex `auth.json`, copy it into the managed live home, save it as a named snapshot, and make that alias active
- `codex-claw account list`: list configured accounts and mark the active one
- `codex-claw account status`: show configured, enabled, and active-account counts
- `codex-claw account enable <alias>` / `disable <alias>`: control whether an account can be selected for switching
- `codex-claw account remove <alias>`: delete a saved snapshot that is not active

These commands manage local runtime state only. They do not add Codex secrets to `config.json`.

If you want to seed the managed live Codex home during onboarding, rerun `codex-claw onboard --import-auth-file <path-to-auth.json>` or accept the guided import prompt when onboarding detects an existing `CODEX_HOME/auth.json`.

## Agents

Keep using the existing agent backbone:

- `agents.defaults.workspace` sets the main workspace root
- `agents.defaults.restrict_to_workspace` keeps writes and most tool access scoped to that workspace by default
- `agents.defaults.allow_read_outside_workspace` relaxes the read-only boundary for file tools when you need to inspect paths outside the workspace
- AGENT/frontmatter `model` stays a raw Codex model id such as `gpt-5.4-mini`
- per-thread runtime commands can override model, thinking, and `fast` without changing static config

Do not rely on fallback arrays in agent config. Active fallback behavior is runtime-owned rather than configured through legacy `fallbacks` lists.

Example:

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.codex-claw/workspace",
      "max_tokens": 128000,
      "max_tool_iterations": 20
    }
  }
}
```

## Workspace and Isolation

Workspace and sandbox knobs sit in a few places. Use them together when you want a tighter operator boundary:

- `agents.defaults.workspace` sets the root workspace path for agents
- `agents.defaults.restrict_to_workspace` keeps writes and most tool access inside that workspace
- `agents.defaults.allow_read_outside_workspace` allows read-only tools to inspect paths outside the workspace when needed
- `tools.allow_read_paths` and `tools.allow_write_paths` add explicit file path allowlists for tool access
- `isolation.enabled` turns on subprocess isolation
- `isolation.expose_paths` lets you pass selected host paths into that isolated environment

Example:

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.codex-claw/workspace",
      "restrict_to_workspace": true,
      "allow_read_outside_workspace": false
    }
  },
  "tools": {
    "allow_read_paths": ["~/shared/read-only"],
    "allow_write_paths": ["~/shared/write"],
    "read_file": {
      "enabled": true,
      "mode": "bytes",
      "max_read_file_size": 65536
    }
  },
  "isolation": {
    "enabled": true,
    "expose_paths": [
      {
        "source": "~/.codex-claw/workspace",
        "target": "/workspace",
        "mode": "rw"
      }
    ]
  }
}
```

## Channels

Telegram and Discord are the supported chat channels. `channels.<name>.enabled` turns each surface on or off, and `allow_from` is the allowlist gate.

The guided onboard flow enables only the surface you selected. The other channel stays disabled until you configure it yourself.

- Keep `allow_from` explicit for personal deployments.
- An empty `allow_from` means open access and triggers a security warning at startup.
- Use `["*"]` only when you want to acknowledge open access on purpose.

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "allow_from": ["YOUR_USER_ID"]
    },
    "discord": {
      "enabled": false
    }
  }
}
```

Store channel tokens in `.security.yml`, not `config.json`. See [security_configuration.md](security_configuration.md) for the matching secret layout.

## Heartbeat

`heartbeat.enabled` and `heartbeat.interval` control the periodic heartbeat loop. The heartbeat watches `workspace/HEARTBEAT.md`, and the detailed task flow lives in [spawn-tasks.md](spawn-tasks.md).

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30
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

`bindings`, hooks, cron, memory, and isolation stay first-class. `tools.mcp` is also part of the main runtime surface, but it is documented with the rest of the tools config.

Use agent-level `mcpServers` allowlists when you want to grant MCP tools to a specific agent. This allowlist is strict: if `mcpServers` is omitted, that agent receives no MCP tools by default.

## Tools

Most operator-facing tool knobs live under `tools.web`, `tools.exec`, `tools.cron`, `tools.skills`, `tools.read_file`, and `tools.mcp`, plus global tool guards such as `filter_sensitive_data`, `filter_min_length`, `allow_read_paths`, `allow_write_paths`, and `media_cleanup`.

Common per-tool enable gates also exist for built-in tools such as `append_file`, `edit_file`, `find_skills`, `install_skill`, `list_dir`, `message`, `send_file`, `spawn`, `subagent`, `web_fetch`, and `write_file`.

```json
{
  "tools": {
    "filter_sensitive_data": true,
    "filter_min_length": 8,
    "web": {
      "enabled": true
    },
    "exec": {
      "enabled": true
    },
    "cron": {
      "enabled": true
    },
    "skills": {
      "enabled": true
    },
    "media_cleanup": {
      "enabled": true
    },
    "read_file": {
      "enabled": true,
      "mode": "bytes"
    },
    "append_file": {
      "enabled": true
    },
    "edit_file": {
      "enabled": true
    },
    "list_dir": {
      "enabled": true
    },
    "message": {
      "enabled": true
    },
    "send_file": {
      "enabled": true
    },
    "spawn": {
      "enabled": true
    },
    "subagent": {
      "enabled": true
    },
    "web_fetch": {
      "enabled": true
    },
    "write_file": {
      "enabled": true
    },
    "mcp": {
      "enabled": false
    }
  }
}
```

See [tools_configuration.md](tools_configuration.md) for the full tool surface, including web search/fetch, exec guardrails, cron, skills, read_file, and MCP discovery/server config.
