## Initialization

Clients must send a single `initialize` request per transport connection before invoking any other method on that connection, then acknowledge with an `initialized` notification. The server returns the user agent string it will present to upstream services, `codexHome` for the server's Codex home directory, and `platformFamily` and `platformOs` strings describing the app-server runtime target; subsequent requests issued before initialization receive a `"Not initialized"` error, and repeated `initialize` calls on the same connection receive an `"Already initialized"` error.

`initialize.params.capabilities` also supports per-connection notification opt-out via `optOutNotificationMethods`, which is a list of exact method names to suppress for that connection. Matching is exact (no wildcards/prefixes). Unknown method names are accepted and ignored.

Applications building on top of `codex app-server` should identify themselves via the `clientInfo` parameter.

**Important**: `clientInfo.name` is used to identify the client for the OpenAI Compliance Logs Platform. If
you are developing a new Codex integration that is intended for enterprise use, please contact us to get it
added to a known clients list. For more context: https://chatgpt.com/admin/api-reference#tag/Logs:-Codex

Example (from OpenAI's official VSCode extension):

```json
{
  "method": "initialize",
  "id": 0,
  "params": {
    "clientInfo": {
      "name": "codex_vscode",
      "title": "Codex VS Code Extension",
      "version": "0.1.0"
    }
  }
}
```

Example with notification opt-out:

```json
{
  "method": "initialize",
  "id": 1,
  "params": {
    "clientInfo": {
      "name": "my_client",
      "title": "My Client",
      "version": "0.1.0"
    },
    "capabilities": {
      "experimentalApi": true,
      "optOutNotificationMethods": ["thread/started", "item/agentMessage/delta"]
    }
  }
}
```

