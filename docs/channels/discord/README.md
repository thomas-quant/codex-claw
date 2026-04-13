# Discord

Discord is one of the two supported chat channels in this fork, alongside Telegram. It connects through the shared PicoClaw channel manager and supports both DMs and server conversations.

## Configuration

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "allow_from": ["YOUR_USER_ID"],
      "group_trigger": {
        "mention_only": false
      }
    }
  }
}
```

| Field         | Type   | Required | Description                                                                 |
| ------------- | ------ | -------- | --------------------------------------------------------------------------- |
| enabled       | bool   | Yes      | Whether to enable the Discord channel                                       |
| allow_from    | array  | No       | Allowlist of user IDs; empty means all users are allowed                    |
| group_trigger | object | No       | Group trigger settings (example: { "mention_only": false })                 |

Store the bot token in `.security.yml`:

```yaml
channels:
  discord:
    token: "YOUR_BOT_TOKEN"
```

## Setup

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications) and create a new application
2. Enable Intents:
   - Message Content Intent
   - Server Members Intent
3. Obtain the Bot Token
4. Put the Bot Token in `.security.yml`
5. Invite the bot to your server and grant the necessary permissions (e.g. Send Messages, Read Message History)
