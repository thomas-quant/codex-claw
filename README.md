# codex-claw

`codex-claw` is a Codex-first personal AI assistant runtime for operators who want a local workspace, direct CLI control, chat surfaces, scheduled jobs, and installable skills in one repo.

It ships as a Go CLI with a shared gateway runtime. You can onboard a workspace, run the agent directly, connect Telegram or Discord, manage Codex account snapshots, schedule recurring tasks, and install or inspect skills from the command line.

## Why It Exists

- Codex-first runtime and config defaults
- local workspace with bundled agent files and built-in skills
- direct CLI commands for onboarding, status, skills, accounts, and cron
- shared gateway for Telegram and Discord chat surfaces
- scheduled jobs stored in the workspace

## Quickstart

### Build or install

```bash
make build
./build/codex-claw version
```

Or install to your local prefix:

```bash
make install
codex-claw version
```

### Initialize a workspace

```bash
codex-claw onboard
```

This creates the config and workspace layout under `~/.codex-claw/` by default. Start from [config/config.example.json](config/config.example.json) when customizing runtime and tool settings.

### Try the core commands

```bash
codex-claw status
codex-claw skills list-builtin
codex-claw account list
codex-claw cron list
```

Start the gateway after configuring your chat surface and secrets:

```bash
codex-claw gateway
```

## Demo Features

### Onboard once, keep the workspace local

`codex-claw onboard` bootstraps the runtime workspace, bundled agent files, and initial config so the rest of the CLI has a stable home directory to work from.

### Run through CLI or chat surfaces

Use the CLI directly, or route the same runtime through Telegram or Discord with the shared gateway. Chat surface setup lives in [docs/chat-apps.md](docs/chat-apps.md).

### Manage Codex accounts

The `account` command group lets you add, import, enable, disable, inspect, and remove Codex account snapshots for runtime switching and operations support.

### Schedule recurring jobs

The `cron` command group writes jobs into the workspace store so reminders and agent-driven recurring work survive restarts. Details: [docs/cron.md](docs/cron.md).

### Discover and install skills

The `skills` command group lists built-in skills, searches registries, installs skills, and shows installed skill details.

## Common Commands

```bash
codex-claw onboard --surface telegram
codex-claw gateway --debug
codex-claw skills search github
codex-claw cron add --name "Daily summary" --message "Summarize today's logs" --cron "0 18 * * *"
codex-claw account status
```

## Repository Map

- `cmd/codex-claw`: main Cobra CLI
- `cmd/membench`: memory benchmark tool
- `pkg/`: runtime packages for agents, channels, config, memory, providers, session, tools, and more
- `docs/`: operator and technical documentation
- `workspace/`: bundled agent files, memory files, and built-in skills
- `config/config.example.json`: reference config

## Development

Go target: `1.25.9`

```bash
make build
make test
make vet
make lint
make check
```

Tests live next to the code they cover. The root `Makefile` is the supported entrypoint for build, verification, install, and release tasks.

## Configuration and Docs

- Chat surfaces: [docs/chat-apps.md](docs/chat-apps.md)
- Tool settings: [docs/tools_configuration.md](docs/tools_configuration.md)
- Security and secrets: [docs/security_configuration.md](docs/security_configuration.md)
- Scheduled jobs: [docs/cron.md](docs/cron.md)
- Docker notes: [docs/docker.md](docs/docker.md)

## License

MIT. See [LICENSE](LICENSE).
