# Codex-Claw Repo Hygiene Handoff

## Current State

- `HEAD`: `9ee5fa3` `chore(codex-claw): finish env contract cleanup`
- Task 4 review is fully closed.
- Task 5 implementation is partially landed in the worktree but **not committed yet**.
- Task 6 has not been fully executed; the final stale-name sweep still has user-facing leftovers.

## Dirty Worktree

These files contain the uncommitted Task 5 hygiene changes:

- `.goreleaser.yaml`
- `docs/docker.md`
- `docs/hardware-compatibility.md`
- `pkg/updater/updater.go`
- `pkg/updater/updater_test.go`
- `scripts/build-macos-app.sh`
- `scripts/setup.iss`

## What Already Happened

### Task 4 follow-up

- Fixed the last two low findings from the env/storage rename pass.
- Commit: `9ee5fa3`
- Review result: approved with no remaining findings in scope.

### Task 5 worker outputs already in tree

- Packaging/build surface cleanup:
  - removed launcher/web-era GoReleaser artifacts
  - retargeted remaining macOS/Windows packaging to `codex-claw`
  - left final repo slug / updater repo target unresolved on purpose
- Updater hardening:
  - removed the implicit `sipeed/picoclaw` release target fallback
  - updater now fails closed when no real release slug is configured
  - added focused updater tests
- Active install docs:
  - `docs/docker.md` now describes local build/run flow
  - `docs/hardware-compatibility.md` no longer points at old release tarballs

## Verification Already Run

- `PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/config ./cmd/picoclaw ./pkg/agent ./pkg/tools -count=1`
- `PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/updater ./cmd/picoclaw -count=1`
- Worker-local checks reported:
  - `go test ./pkg/updater`
  - `bash -n scripts/build-macos-app.sh`
  - grep checks for removed launcher/web packaging identifiers

## Highest-Priority Remaining Hygiene Work

### 1. Review and commit the current Task 5 changes

- Inspect the seven dirty files listed above.
- If acceptable, commit them as one or more hygiene commits.
- Suggested split:
  - packaging/installer surface
  - updater fail-closed behavior
  - active install docs

### 2. Finish Task 6 active-surface cleanup

The stale-name sweep still found user-visible leftovers outside the committed/dirty Task 5 scope:

- `AGENTS.md`
  - still says `cmd/picoclaw` is the main CLI and `make build` builds `picoclaw`
- `Makefile`
  - Docker compose service/image targets still use `picoclaw-agent`, `picoclaw-gateway`, `picoclaw:latest`, `picoclaw:full`
- `.env.example`
  - still contains old env-prefix examples
- CLI/help examples
  - `cmd/picoclaw/internal/cron/{enable,disable,remove}.go`
  - `cmd/picoclaw/internal/skills/{install,list,show,remove,listbuiltin,installbuiltin}.go`
  - `pkg/commands/cmd_start.go`
  - `cmd/membench/main.go`
- Docker test script
  - `scripts/test-docker-mcp.sh`
- Active localized channel docs still on old branding
  - `docs/channels/telegram/README.zh.md`
  - `docs/channels/discord/README.zh.md`
  - `docs/channels/discord/README.fr.md`
  - `docs/channels/discord/README.pt-br.md`
  - `docs/channels/discord/README.ja.md`
  - `docs/channels/discord/README.vi.md`

## Things Intentionally Deferred

These should **not** be treated as hygiene misses in the next thread unless the scope changes:

- Go module path rename from `github.com/sipeed/picoclaw`
- source directory rename from `cmd/picoclaw`
- internal symbol/package names that are not user-facing
- compatibility-sensitive internals such as credential HKDF strings unless explicitly revisited
- final release repo slug / homepage URL selection

## Possibly Deferred But Worth Deciding

- `.goreleaser.yaml` and `scripts/setup.iss` may still contain old homepage/repo URLs because the final remote slug is not chosen yet.
- `pkg/tools/web.go` still advertises `https://github.com/sipeed/picoclaw` in the user agent.
- `pkg/providers/factory_provider.go` still uses `PicoClaw/%s` as a provider user-agent string.

## Recommended Next-Thread Sequence

1. Review `git diff` for the seven dirty Task 5 files.
2. Commit Task 5 cleanup.
3. Sweep the remaining Task 6 user-facing leftovers listed above.
4. Re-run:
   - `rg -n --glob '!.git/**' --glob '!.planning/**' --glob '!reference/**' --glob '!docs/history/**' 'picoclaw|PicoClaw|PICOCLAW_|\\.picoclaw|sipeed/picoclaw|ghcr.io/.*/picoclaw|docker.io/.*/picoclaw' .`
   - `PATH=/tmp/go-toolchain/go/bin:$PATH go test ./cmd/picoclaw/... ./pkg/config ./pkg/credential ./pkg/pid ./pkg/agent ./pkg/tools ./pkg/updater -count=1`
   - `PATH=/tmp/go-toolchain/go/bin:$PATH go test ./... -count=1`
5. Decide whether the remaining repo-slug URLs stay deferred or get a temporary explicit placeholder policy.
