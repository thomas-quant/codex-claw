# Telegram

Telegram is one of the two supported chat channels, alongside Discord. It uses the Telegram Bot API with long polling and connects through the shared codex-claw channel manager.

## Configuration

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "allow_from": ["123456789"],
      "proxy": "",
      "use_markdown_v2": false
    }
  }
}
```

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| enabled | bool | Yes | Enable the Telegram channel |
| allow_from | array | No | Optional allowlist of Telegram user IDs |
| proxy | string | No | Optional proxy URL for Telegram API access |
| use_markdown_v2 | bool | No | Enable Telegram MarkdownV2 formatting |

Store the token in `.security.yml`:

```yaml
channels:
  telegram:
    token: "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
```

## Behavior

- Supports text messages, media attachments, replies, and command handling.
- Uses the shared channel allowlist and thread identity rules from codex-claw.
- Keeps the guide focused on messaging and command handling; voice and other removed provider-era surfaces are out of scope here.

## Commands

Telegram exposes the same thread-level command surface as the rest of codex-claw for permitted users, including model and runtime controls such as `/set model`, `/set thinking`, `/fast`, `/compact`, and `/reset`.

## Setup

1. Create a bot with `@BotFather`.
2. Put the HTTP API token in `.security.yml`.
3. Set `allow_from` if you want to restrict who can talk to the bot.
