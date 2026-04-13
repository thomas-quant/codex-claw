# ASR (Automatic Speech Recognition)

Voice input is deferred in the current Codex-first fork.

This package is kept as a historical note for deployments that add an explicit runtime-native voice stack, but it is not a supported active setup path in PicoClaw today.

## What Changed

- Do not expect ASR to be configured through the legacy voice catalog.
- Do not rely on auto-discovery or provider-era setup flows.
- Treat voice support as a separate, optional runtime concern.

## Current Guidance

If your deployment reintroduces ASR, treat it as an optional runtime-native feature with its own config surface and secrets handling.

The active fork does not provide a Codex-first ASR setup flow, so there is nothing to choose from here today.
