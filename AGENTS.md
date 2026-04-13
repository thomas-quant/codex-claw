# Repository Guidelines

## Project Structure & Module Organization
`cmd/picoclaw` is the main Cobra CLI and `cmd/membench` is the benchmark tool. Core runtime code lives under `pkg/`, with domain packages such as `agent`, `channels`, `commands`, `config`, `credential`, `memory`, `providers`, `session`, and `tools`; tests live beside the code. Operator docs live in `docs/`, archived notes in `docs/history/`, planning artifacts in `.planning/`, reference material in `reference/`, and companion tooling in `support/`. Use `config/config.example.json` and `workspace/` as the reference for operator config and bundled agent files. Localized channel docs live next to the English channel guides under `docs/channels/`.

## Build, Test, and Development Commands
Use the root `Makefile` first:

- `make build`: build the main `picoclaw` binary.
- `make test`: run `go generate`, tagged Go tests, and repo verification steps wired into the Makefile.
- `make vet`, `make lint`, `make fmt`, `make fix`, `make check`: static analysis, formatting, autofix, and full verification.
- `make build-all`: build release binaries for the supported target set.
- `make install` / `make uninstall`: install or remove the CLI from your local prefix.

## Coding Style & Naming Conventions
Go targets `1.25.9`. Let `golangci-lint fmt` manage formatting (`gofumpt`, `goimports`, `golines`); keep lines within 120 columns and prefer `any` over `interface{}`. Package names stay lowercase. Platform-specific files use suffixes such as `_unix.go` and `_windows.go`. In `cmd/picoclaw`, top-level Cobra constructors follow `NewXCommand()`, while leaf commands use unexported `newXCommand()` in matching files. Keep docs and user-facing strings aligned with the current Codex-first runtime and supported channel set.

## Testing Guidelines
Keep tests adjacent to implementation as `*_test.go`. Prefer descriptive names like `TestFoo_Bar`, table-driven `t.Run(...)`, `t.TempDir()`, and `httptest` when needed. Run `make test` before opening a PR, and use narrower `go test` package targets while iterating. CI runs `go generate ./...`, `golangci-lint`, `govulncheck`, and `go test -tags goolm,stdjson ./...`.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit style: `feat(cli): ...`, `fix(gemini): ...`, `build: ...`. Keep subjects imperative and scoped when useful. PRs should include a short description, related issue (`Fixes #123`) when applicable, technical context for non-doc changes, the test environment, and logs or screenshots for user-facing CLI or channel behavior changes. Update docs or config examples when workspace, hooks, channels, or runtime behavior changes.

## Security & Configuration Tips
Never commit real secrets; start from `.env.example` and `config/config.example.json`. Keep channel `allow_from` restrictions explicit, prefer localhost bindings for gateway services, and route subprocess-spawning features through `pkg/isolation`.
