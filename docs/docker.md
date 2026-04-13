# Docker

## Gateway Profile

codex-claw does not publish a launcher image or a separate browser-facing Docker package. Run Docker from a local checkout of this repo and build the images locally:

```bash
git clone <your codex-claw fork or mirror>
cd <repo-dir>
make docker-build
docker compose -f docker/docker-compose.yml --profile gateway up
```

On first run, create or edit `docker/data/config.json`, then start the gateway in the background:

```bash
docker compose -f docker/docker-compose.yml --profile gateway up -d
docker compose -f docker/docker-compose.yml logs -f codex-claw-gateway
```

If you need host access to the gateway port, set `CODEX_CLAW_GATEWAY_HOST=0.0.0.0`.

Use `make docker-build-full` with `docker/docker-compose.full.yml` if you need the full image variant with broader MCP runtime support.

## Agent Profile

For one-shot or interactive local runs:

```bash
docker compose -f docker/docker-compose.yml run --rm codex-claw-agent -m "What is 2+2?"
docker compose -f docker/docker-compose.yml run --rm codex-claw-agent
```

## Config Shape

Use the Codex-first `runtime` block inside `docker/data/config.json`:

```json
{
  "runtime": {
    "codex": {
      "default_model": "gpt-5.4"
    },
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

Provide `DEEPSEEK_API_KEY` through your container environment if you want the HTTP fallback to be usable.

There are no published legacy images for this distribution path. Build from the checked-out repo, then use the gateway, CLI, Telegram, or Discord surfaces directly. There is no launcher or browser UI.
