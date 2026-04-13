# Credential Encryption

Encrypted credentials still work for any `SecureString` value loaded from `config.json` or `.security.yml`. This is useful for channel tokens or tool secrets that you do not want stored as plaintext on disk.

## Supported Formats

Any supported secret field can use one of these forms:

| Format | Example | Behavior |
|--------|---------|----------|
| Plaintext | `discord-token` | Used as-is |
| File reference | `file://discord.token` | Reads the file relative to the config directory |
| Encrypted | `enc://AAAA...` | Decrypted at startup |

Codex auth is outside this system. DeepSeek fallback credentials are expected from `DEEPSEEK_API_KEY`.

## Encrypted Values

`enc://` values are decrypted with:

- `PICOCLAW_KEY_PASSPHRASE`
- an SSH private key, either from `PICOCLAW_SSH_KEY_PATH` or the default key path

The current implementation uses AES-256-GCM with HKDF-SHA256-derived keys. The stored wire format remains:

```text
enc://<base64(salt + nonce + ciphertext)>
```

## Practical Use

This fork no longer stores model/provider secrets in config, so the common cases are:

- Telegram or Discord bot tokens
- web tool API keys
- skill registry tokens

Example:

```yaml
channels:
  telegram:
    token: "enc://AAAA..."
```

## Notes

- Keep `PICOCLAW_KEY_PASSPHRASE` out of shell history and version control.
- `file://` is often simpler than `enc://` for local deployments that already use secret files.
- Existing encrypted values remain valid; the cleanup pass did not remove credential decryption support.
