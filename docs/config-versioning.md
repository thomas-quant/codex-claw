# Config Schema Versioning Guide

Config versioning still matters, but it no longer preserves the removed provider catalog. The current schema is a deliberate hard break toward a Codex-first runtime.

## Current Direction

- runtime defaults live under `runtime`
- Codex is the primary runtime
- DeepSeek is the only built-in fallback
- `model_list`, `providers`, and old auth-era config branches are no longer supported

If a config still depends on those deleted sections, load should fail fast instead of applying partial compatibility logic.

## What Versioning Still Covers

Versioning is still useful for:

- channel config normalization
- workspace and agent defaults
- tool and MCP defaults
- backup creation before schema-changing writes

It is not meant to keep the removed provider catalog alive.

## Practical Rules

1. Increment `CurrentVersion` when a breaking on-disk schema change is introduced.
2. Keep migrations narrow and explicit.
3. Prefer hard failure over silent best-effort conversion when old provider/auth fields are involved.
4. Create backups before overwriting `config.json` or `.security.yml`.
5. Keep `defaults.go` aligned with the latest schema.

## Backups

Before writing a migrated config, the loader should preserve the previous on-disk state with date-stamped backup files. That gives operators a clean rollback path when a schema change goes wrong.

## When To Add A Migration

Add a real migration only when the product still intends to preserve that feature surface. Examples:

- renaming a surviving channel field
- restructuring the `runtime` block
- changing MCP or tool config layout

Do not add migrations for removed provider or auth-era config.

## Troubleshooting

If config loading fails after an upgrade:

- check for deleted keys such as `model_list` or `providers`
- compare against `config/config.example.json`
- restore from the backup file if you need to recover a working baseline quickly
