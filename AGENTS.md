# Repository Guidelines

## Project Structure & Module Organization
`cmd/picoclaw` is the Cobra CLI, `cmd/picoclaw-launcher-tui` the terminal launcher, and `cmd/membench` the benchmark tool. `pkg/` holds the core runtime by domain, including `agent`, `tools`, `providers`, `channels`, `config`, `memory`, and `session`; tests live beside the code. `web/backend` serves embedded launcher assets, while `web/frontend` is the React 19 + Vite UI. Use `config/config.example.json` and `workspace/` as the reference for operator config and agent files. User docs live in `docs/`, with localized docs under `docs/zh`, `docs/fr`, `docs/ja`, `docs/pt-br`, `docs/vi`, and `docs/my`.

## Build, Test, and Development Commands
Use the root `Makefile` first:

- `make build`: build the main `picoclaw` binary.
- `make build-launcher` / `make build-launcher-tui`: build the web or TUI launcher.
- `make test`: run `go generate`, tagged Go tests, then `web` checks.
- `make vet`, `make lint`, `make fmt`, `make fix`, `make check`: static analysis, formatting, autofix, and full verification.
- `cd web && make dev`: run the launcher backend and Vite frontend together. Use `make dev-frontend` or `make dev-backend` only when isolating one side.

## Coding Style & Naming Conventions
Go targets `1.25.9`. Let `golangci-lint fmt` manage formatting (`gofumpt`, `goimports`, `golines`); keep lines within 120 columns and prefer `any` over `interface{}`. Package names stay lowercase. Platform-specific files use suffixes such as `_unix.go` and `_windows.go`. In `cmd/picoclaw`, top-level Cobra constructors follow `NewXCommand()`, while leaf commands use unexported `newXCommand()` in matching files. Frontend code uses 2-space indentation, no semicolons, sorted imports, and `pnpm check` for fixes.

## Testing Guidelines
Keep tests adjacent to implementation as `*_test.go`. Prefer descriptive names like `TestFoo_Bar`, table-driven `t.Run(...)`, `t.TempDir()`, and `httptest` when needed. Run `make test` before opening a PR; for launcher changes, also use `cd web && make test` or `pnpm lint` during iteration. CI runs `go generate ./...`, `golangci-lint`, `govulncheck`, and `go test -tags goolm,stdjson ./...`.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit style: `feat(cli): ...`, `fix(gemini): ...`, `build: ...`. Keep subjects imperative and scoped when useful. PRs should follow `.github/pull_request_template.md`: include a short description, change type, related issue (`Fixes #123`), technical context for non-doc changes, test environment, and logs or screenshots for UI, launcher, or channel behavior changes. Update docs or config examples when workspace, hooks, or channel flows change.

## Security & Configuration Tips
Never commit real secrets; start from `.env.example` and `config/config.example.json`. Keep channel `allow_from` restrictions explicit, prefer localhost bindings for gateway services, and route subprocess-spawning features through `pkg/isolation`.
