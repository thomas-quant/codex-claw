## Skills

Compressed technical summary of skills, apps, auth/account endpoints, and experimental API opt-in.

### Invoking Skills

- Skills can be invoked inline with `$<skill-name>` in text input.
- Recommended: also send an explicit `skill` input item so resolution is deterministic and low-latency.

Minimal shape:

```json
{
  "method": "turn/start",
  "params": {
    "threadId": "thread-1",
    "input": [
      { "type": "text", "text": "$skill-creator Add a new skill." },
      { "type": "skill", "name": "skill-creator", "path": "/abs/path/SKILL.md" }
    ]
  }
}
```

### Skill Discovery And Config

- `skills/list`:
  - Lists skills for provided `cwds`.
  - Supports `forceReload` and `perCwdExtraUserRoots`.
  - May use per-cwd cache unless `forceReload: true`.
- `skills/changed`:
  - Notification emitted when watched local skill files change.
  - Treat as invalidation signal and re-run `skills/list`.
- `skills/config/write`:
  - Enables/disables a skill by absolute `path` or logical `name`.

## Apps

### App Discovery

- `app/list` returns connector/app metadata including:
  - `id`, `name`, `description`
  - branding/logo fields
  - `installUrl`
  - `labels`, `appMetadata`
  - `isAccessible`, `isEnabled`
- Supports pagination (`cursor`, `limit`) and `forceRefetch`.
- If `threadId` is provided, feature gating is evaluated against that thread's config snapshot.
- `app/list/updated` notifications are emitted as accessible and directory app sources finish loading.

### App Invocation

- You can mention app slug in text (`$demo-app` style).
- Recommended: include explicit `mention` item with canonical app URI:
  - `app://<connector-id>`
- Plugin mentions use:
  - `plugin://<plugin-name>@<marketplace-name>`

Minimal shape:

```json
{
  "method": "turn/start",
  "params": {
    "threadId": "thread-1",
    "input": [
      { "type": "text", "text": "$demo-app Pull latest updates." },
      { "type": "mention", "name": "Demo App", "path": "app://demo-app" }
    ]
  }
}
```

## Auth Endpoints

The auth/account surface includes request/response methods and server notifications.

### Supported Auth Modes

- `apiKey`:
  - Start login with `account/login/start` and `type: "apiKey"`.
- `chatgpt` (recommended managed mode):
  - Start with `type: "chatgpt"` (browser flow) or `type: "chatgptDeviceCode"` (device code flow).
  - Codex persists and refreshes tokens.

Current mode is surfaced by `account/updated` (`authMode`) and available via `account/read`.

### Endpoint And Notification Reference

- `account/read`:
  - Reads current account state.
  - Optional `refreshToken: true` forces token refresh.
  - Returns `requiresOpenaiAuth` indicating whether current provider requires OpenAI credentials.
- `account/login/start`:
  - Begins login flow (`apiKey`, `chatgpt`, `chatgptDeviceCode`).
- `account/login/completed` (notification):
  - Emitted when login attempt finishes (success/error).
- `account/login/cancel`:
  - Cancels pending managed ChatGPT login by `loginId`.
- `account/logout`:
  - Signs out and triggers `account/updated`.
- `account/updated` (notification):
  - Emitted on auth-mode changes; includes `authMode` (`apikey`, `chatgpt`, or `null`) and optional `planType`.
- `account/rateLimits/read`:
  - Reads ChatGPT rate limits.
- `account/rateLimits/updated` (notification):
  - Emitted when rate limits change.
- `mcpServer/oauthLogin/completed` (notification):
  - Emitted after `mcpServer/oauth/login` flow for a configured MCP server.
- `mcpServer/startupStatus/updated` (notification):
  - Startup transitions for configured MCP servers (`starting`, `ready`, `failed`, `cancelled`).

### Rate Limits Payload Notes

Rate limit entries can include values such as:

- usage fraction (`usedPercent`)
- window length (`windowDurationMins`)
- reset timestamp (`resetsAt`, Unix time)

## Experimental API Opt-in

Some methods/fields are intentionally unstable and gated by capability.

### Stable vs Experimental Schema Generation

- Stable-only schemas (default):
  - `codex app-server generate-ts --out DIR`
  - `codex app-server generate-json-schema --out DIR`
- Include experimental schema entries:
  - add `--experimental` to generation commands.

### Runtime Opt-in

Client must opt in during initialization:

```json
{
  "method": "initialize",
  "params": {
    "clientInfo": { "name": "my-client", "version": "1.0.0" },
    "capabilities": { "experimentalApi": true }
  }
}
```

Notes:

- Capability is negotiated once per connection during `initialize`.
- Repeated `initialize` on same connection is invalid.

### Behavior Without Opt-in

- Calls using experimental-only methods/fields are rejected.
- Typical message pattern: `<descriptor> requires experimentalApi capability`.

### Maintainer Gating Pattern

Experimental surface is marked in protocol/types and propagated through generated schema and tests. Keep fixtures/tests regenerated when adding new experimental methods or fields.

