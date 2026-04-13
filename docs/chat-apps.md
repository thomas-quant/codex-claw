# Chat Apps Configuration

The supported chat surfaces are:

- [Telegram](channels/telegram/README.md)
- [Discord](channels/discord/README.md)

Both run through the shared channel manager and start with:

```bash
picoclaw gateway
```

## Telegram

Recommended if you want the lightest personal deployment.

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "allow_from": ["YOUR_USER_ID"],
      "use_markdown_v2": false
    }
  }
}
```

Put the bot token in `.security.yml`:

```yaml
channels:
  telegram:
    token: "YOUR_BOT_TOKEN"
```

Telegram keeps the shared command surface, including runtime controls such as `/set model`, `/set thinking`, `/fast`, `/compact`, `/reset`, and `/status`.

## Discord

Use Discord when you want the same runtime in DMs or servers.

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

Put the bot token in `.security.yml`:

```yaml
channels:
  discord:
    token: "YOUR_BOT_TOKEN"
```

If you only want replies on mention or a custom prefix, configure the Discord trigger options in `config.json`.

## Notes

- Active operator docs cover Telegram and Discord only.
- Keep `allow_from` explicit on both Telegram and Discord for personal deployments.
- Channel startup errors are easiest to diagnose by running `picoclaw gateway` directly in the terminal.
