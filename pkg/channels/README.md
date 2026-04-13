# Channel System

`pkg/channels` contains the shared channel manager, capability interfaces, and the Telegram/Discord channel implementations that remain in this fork.

## Fork Boundary

- Only `telegram` and `discord` are registered and supported.
- The generic manager stays in place for routing, retries, allowlists, typing, placeholders, and outbound splitting.
- Channel families outside the retained Telegram/Discord boundary are out of scope here.

## Runtime Shape

- Inbound messages enter the bus as structured chat events.
- The agent loop produces outbound text, media, and control updates.
- The manager dispatches those updates to the active channel implementation.

## Channel Responsibilities

Each channel package owns transport-specific work:

- connect and reconnect to the platform API
- translate platform events into PicoClaw bus messages
- send text and media replies
- enforce channel allowlists and thread identity rules
- surface optional capabilities such as typing, placeholders, message edits, and reactions when the platform supports them

## Extending the System

To add or change a channel, update the package under `pkg/channels/<name>/`, register it through the shared factory registry, and wire it into the manager. Keep platform-specific logic in the channel package and shared behavior in the manager.
