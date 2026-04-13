# Docker

## Gateway Profile

Run the gateway without installing locally:

```bash
git clone https://github.com/sipeed/picoclaw.git
cd picoclaw
docker compose -f docker/docker-compose.yml --profile gateway up
```

On first run, create or edit `docker/data/config.json`, then start the gateway in the background:

```bash
docker compose -f docker/docker-compose.yml --profile gateway up -d
docker compose -f docker/docker-compose.yml logs -f picoclaw-gateway
```

If you need host access to the gateway port, set `CODEX_CLAW_GATEWAY_HOST=0.0.0.0`.

## Agent Profile

For one-shot or interactive local runs:

```bash
docker compose -f docker/docker-compose.yml run --rm picoclaw-agent -m "What is 2+2?"
docker compose -f docker/docker-compose.yml run --rm picoclaw-agent
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

There is no launcher or browser UI in this fork. Use the gateway, CLI, Telegram, or Discord surfaces directly.
