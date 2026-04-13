# Codex-Claw Repo Hygiene Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the repository read, build, and ship as `codex-claw` instead of a half-renamed PicoClaw fork, while separating active product docs from historical/reference material.

**Architecture:** Treat this as a transactional hygiene pass, not a string-replace spree. First separate active product surfaces from history/reference debris, then rename the active runtime/operator surface as one unit, then clean packaging and release metadata so binaries, home-dir/env contracts, docs, and updater behavior stop disagreeing with each other.

**Tech Stack:** Go, Cobra, Markdown docs, Makefile, GoReleaser, shell packaging scripts

---

## Assumptions

- Canonical public product name is `codex-claw`.
- The launcher/web product surface is gone and should not be revived during hygiene.
- The storage/env contract can hard-cut to new names; this pass does **not** preserve `PICOCLAW_*` or `~/.picoclaw` compatibility unless explicitly added later.
- `codex-account-manager` stays in the repo for now, but as a companion tool rather than part of the core product tree.
- The final remote/module slug is **not** chosen yet, so the Go module path rename is tracked as a deferred second-wave change rather than part of this pass.

## File Structure

### Create

- `.planning/` — internal planning and deferred-work bucket
- `.planning/superpowers/` — moved plans/specs/history from `docs/superpowers`
- `docs/history/` — archived historical design notes that should stay in-repo but not read as active docs
- `reference/codex-app-server/` — Codex app-server reference notes currently dumped at repo root
- `reference/openclaw/` — archived OpenClaw reference payload
- `support/codex-account-manager/` — companion auth tool moved out of the root product surface

### Move / Rename

- `docs/superpowers/**` -> `.planning/superpowers/**`
- `followups.md` -> `.planning/followups.md`
- `codex-app-server-harness-spec.md` -> `reference/codex-app-server/harness-spec.md`
- `codex-app-serv.md` -> `reference/codex-app-server/official-summary.md`
- `codex-app-serv.part-01.md` -> `reference/codex-app-server/official-summary.part-01.md`
- `codex-app-serv.part-02.md` -> `reference/codex-app-server/official-summary.part-02.md`
- `codex-app-serv.part-03.md` -> `reference/codex-app-server/official-summary.part-03.md`
- `codex-app-serv.part-04.md` -> `reference/codex-app-server/official-summary.part-04.md`
- `codex-app-serv.part-05.md` -> `reference/codex-app-server/official-summary.part-05.md`
- `openclaw-stuff/` -> `reference/openclaw/`
- `codex-account-manager/` -> `support/codex-account-manager/`

### Delete

- `.codex`
- `triage-issue.json`
- `triage-pr.json`
- `scripts/test-irc.sh`
- `docs/channels/matrix/`
- `pkg/auth/` if it remains empty
- `pkg/migrate/` if it remains empty/unreferenced after prior cleanup
- stale launcher packaging assets if they are no longer part of the product surface

### Modify

- `AGENTS.md`
- `Makefile`
- `.goreleaser.yaml`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/internal/helpers.go`
- `cmd/picoclaw/internal/agent/helpers.go`
- `cmd/picoclaw/internal/gateway/command.go`
- `cmd/picoclaw/internal/onboard/helpers.go`
- `cmd/picoclaw/internal/skills/helpers.go`
- `cmd/picoclaw/internal/version/command.go`
- `cmd/picoclaw/internal/status/command.go`
- `cmd/picoclaw/internal/cliui/onboard.go`
- `cmd/picoclaw/internal/cliui/status.go`
- `pkg/env.go`
- `pkg/config/envkeys.go`
- `pkg/config/defaults.go`
- `pkg/pid/pidfile.go`
- `pkg/credential/credential.go`
- `pkg/credential/keygen.go`
- `pkg/agent/context.go`
- `pkg/tools/web.go`
- `pkg/providers/error_classifier.go`
- `pkg/providers/cooldown.go`
- active operator docs under `docs/`
- any links into moved planning/reference/history locations

### Keep As-Is

- `workspace/` templates
- `cmd/membench/`
- core runtime packages under `pkg/`
- active Telegram/Discord channel docs after wording cleanup
- historical notes that are intentionally moved under `docs/history/` or `reference/`

## Task 1: Establish Repo Taxonomy And Remove Root Noise

**Files:**
- Create: `.planning/`, `.planning/superpowers/`, `docs/history/`, `reference/codex-app-server/`, `reference/openclaw/`, `support/`
- Move: `docs/superpowers/**`, `followups.md`, root `codex-app*` files, `openclaw-stuff/`, `codex-account-manager/`
- Delete: `.codex`, `triage-issue.json`, `triage-pr.json`, `scripts/test-irc.sh`, `docs/channels/matrix/`, empty `pkg/auth/`, empty `pkg/migrate/`
- Modify: links in any active doc that points at the moved files

- [ ] Create the non-product buckets first so moves are one-way and obvious:
  - `.planning/` for live engineering artifacts
  - `docs/history/` for archived internal design notes
  - `reference/` for external/reference material that should stay searchable but not look like product docs
- [ ] Move `docs/superpowers/**` into `.planning/superpowers/**` and move `followups.md` to `.planning/followups.md`.
- [ ] Move the root Codex reference files into `reference/codex-app-server/` with the renamed filenames listed above.
- [ ] Move `openclaw-stuff/` to `reference/openclaw/` and remove its nested `.git` if it still exists after the move.
- [ ] Move `codex-account-manager/` to `support/codex-account-manager/` and prune obvious local-development junk there (`.venv`, `.pytest_cache`, `.worktrees`) if it should stay versioned as source only.
- [ ] Delete root noise and dead leftovers: `.codex`, `triage-issue.json`, `triage-pr.json`, `scripts/test-irc.sh`, `docs/channels/matrix/`, and any empty `pkg/auth/` / `pkg/migrate/` trees.
- [ ] Add a short “historical design note; not normative product documentation” banner to any design doc kept under `docs/history/`.
- [ ] Run:

```bash
find . -maxdepth 2 -mindepth 1 | sort
rg -n 'docs/superpowers|followups\\.md|openclaw-stuff|codex-account-manager|codex-app-serv|codex-app-server-harness-spec' .
```

Expected: moved content is no longer living at repo root or under active `docs/`, and deleted root-noise files no longer appear.

## Task 2: Rewrite Contributor And Active Docs Surface

**Files:**
- Modify: `AGENTS.md`
- Modify: active operator docs under `docs/`, especially:
  - `docs/configuration.md`
  - `docs/providers.md`
  - `docs/chat-apps.md`
  - `docs/troubleshooting.md`
  - `docs/config-versioning.md`
  - `docs/security_configuration.md`
  - `docs/channels/telegram/README.md`
  - `docs/channels/discord/README.md`
- Modify: `pkg/providers/error_classifier.go`
- Modify: `pkg/providers/cooldown.go`

- [ ] Rewrite `AGENTS.md` so it matches the actual tree: no launcher/web paths, no deleted localized-doc claims, no removed commands.
- [ ] Rewrite active docs so they describe the current product directly instead of narrating a fork transition. Remove repeated phrases like “this fork,” “pre-fork provider catalog,” and “forked runtime” from active operator pages.
- [ ] Keep removed/upstream-surface discussion only where it changes operator behavior; otherwise move it to a single compatibility/history note.
- [ ] Move contradictory old design docs like `docs/design/provider-refactoring*.md` into `docs/history/` if they are kept at all.
- [ ] Reword adjacent code comments that currently imply active parity with OpenClaw; attribute them as inherited heuristics instead.
- [ ] Remove stale `OAuth/Token Auth` wording from CLI status/onboard surfaces if the runtime no longer exposes that auth model.
- [ ] Run:

```bash
rg -n 'this fork|pre-fork|forked runtime|OAuth/Token Auth|launcher|web UI|matrix|slack|whatsapp|feishu' AGENTS.md docs cmd/picoclaw/internal pkg/providers
```

Expected: active docs and adjacent user-facing text no longer read like a transitional fork or advertise deleted surfaces.

## Task 3: Perform The User-Facing Codex-Claw Identity Cut

**Files:**
- Modify: `Makefile`
- Modify: `cmd/picoclaw/main.go`
- Modify: `cmd/picoclaw/internal/version/command.go`
- Modify: `cmd/picoclaw/internal/status/command.go`
- Modify: `cmd/picoclaw/internal/gateway/command.go`
- Modify: `cmd/picoclaw/internal/onboard/command.go`
- Modify: `cmd/picoclaw/internal/cliui/onboard.go`
- Modify: `cmd/picoclaw/internal/cliui/status.go`
- Modify: `pkg/env.go`
- Modify: `pkg/agent/context.go`
- Modify: `pkg/tools/web.go`
- Modify: active docs that still show `picoclaw` commands/branding

- [ ] Set the canonical visible product name to `codex-claw` everywhere users see it:
  - Cobra root command `Use`
  - short/long help text
  - version/status/onboard command text
  - app constants in `pkg/env.go`
  - agent self-identification strings in prompt context
  - HTTP user-agent branding in `pkg/tools/web.go`
- [ ] Update `Makefile` so the built artifact is `codex-claw`, while decoupling source directory selection from the binary name if `cmd/picoclaw` is not renamed yet.
- [ ] Update active docs and examples so operator commands use `codex-claw ...` instead of `picoclaw ...`.
- [ ] Keep the Go module path and source directory rename out of this task; this is the outward identity cut only.
- [ ] Run:

```bash
rg -n --glob '!docs/history/**' --glob '!.planning/**' --glob '!reference/**' 'picoclaw|PicoClaw' cmd pkg docs Makefile AGENTS.md config
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./cmd/picoclaw ./pkg/env.go ./pkg/agent ./pkg/tools -count=1
```

Expected: remaining `picoclaw` hits are limited to intentionally deferred code-internal/module-path references, not user-facing strings.

## Task 4: Rename The Storage And Operator Contract In One Pass

**Files:**
- Modify: `pkg/config/envkeys.go`
- Modify: `pkg/config/defaults.go`
- Modify: `cmd/picoclaw/internal/helpers.go`
- Modify: `cmd/picoclaw/internal/agent/helpers.go`
- Modify: `cmd/picoclaw/internal/onboard/helpers.go`
- Modify: `cmd/picoclaw/internal/skills/helpers.go`
- Modify: `pkg/pid/pidfile.go`
- Modify: `pkg/credential/credential.go`
- Modify: `pkg/credential/keygen.go`
- Modify: `config/config.example.json`
- Modify: active docs under `docs/`

- [ ] Hard-cut the storage/env naming from PicoClaw to Codex-Claw:
  - `PICOCLAW_HOME` -> `CODEX_CLAW_HOME`
  - `PICOCLAW_CONFIG` -> `CODEX_CLAW_CONFIG`
  - `~/.picoclaw` -> `~/.codex-claw`
  - `.picoclaw.pid` -> `.codex-claw.pid`
  - `.picoclaw_history` -> `.codex-claw_history`
  - `picoclaw_ed25519.key` -> `codex-claw_ed25519.key`
- [ ] Update onboarding, config lookup, pid handling, credential key discovery, and builtin-skills path handling to use the new canonical locations.
- [ ] Update `config/config.example.json` and every active operator doc that shows home-dir/env/config examples.
- [ ] Do **not** add dual-read compatibility unless you explicitly decide to support a migration path later.
- [ ] Run:

```bash
rg -n --glob '!docs/history/**' --glob '!.planning/**' --glob '!reference/**' 'PICOCLAW_|\\.picoclaw|picoclaw_ed25519|picoclaw_history' .
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./cmd/picoclaw/internal/... ./pkg/config ./pkg/pid ./pkg/credential -count=1
```

Expected: the old env/home/key/history names are gone from active product surfaces.

## Task 5: Clean Build, Release, And Installer Surfaces

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `Makefile`
- Modify: `pkg/updater/updater.go`
- Modify: `scripts/build-macos-app.sh`
- Modify: `scripts/setup.iss`
- Modify: active install/release docs such as `docs/docker.md`, `docs/hardware-compatibility.md`, `docs/debug.md`

- [ ] Remove launcher-specific packaging/build artifacts from `.goreleaser.yaml`, `scripts/build-macos-app.sh`, and `scripts/setup.iss` if the launcher is truly gone.
- [ ] Rename core release/package outputs to `codex-claw` where packaging remains.
- [ ] Stop pointing updater/release docs at `sipeed/picoclaw`. If the new release repo slug is not ready, disable self-update cleanly or make it explicitly unconfigured rather than silently targeting the old repo.
- [ ] Update container/image/archive/install docs to match the actual post-cleanup distribution story.
- [ ] Run:

```bash
rg -n 'picoclaw-launcher|PicoClaw Launcher|sipeed/picoclaw|ghcr.io/.*/picoclaw|docker.io/.*/picoclaw' Makefile .goreleaser.yaml scripts docs pkg/updater
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/updater ./cmd/picoclaw -count=1
```

Expected: packaging and updater surfaces no longer advertise removed launcher assets or fetch the old project release stream.

## Task 6: Final Hygiene Verification

**Files:**
- Modify any touched files from Tasks 1-5

- [ ] Run the active-surface stale-name sweep:

```bash
rg -n --glob '!.git/**' --glob '!.planning/**' --glob '!reference/**' --glob '!docs/history/**' 'picoclaw|PicoClaw|PICOCLAW_|\\.picoclaw' .
```

Expected: only intentionally deferred internals remain, chiefly the Go module/import path and any explicitly archived history/reference content.

- [ ] Run the focused verification sweep:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./cmd/picoclaw/... ./pkg/config ./pkg/credential ./pkg/pid ./pkg/agent ./pkg/tools ./pkg/updater -count=1
```

- [ ] Run the broad guardrail sweep:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./... -count=1
```

- [ ] Confirm the root tree now reads as:
  - product code/config/docs at root
  - planning under `.planning/`
  - historical docs under `docs/history/`
  - external/reference material under `reference/`
  - companion auth tool under `support/`

## Deferred Second-Wave Work

These are intentionally **not** part of this hygiene pass:

- Rename `go.mod` from `github.com/sipeed/picoclaw` to the final `codex-claw` module path once the real remote slug exists.
- Rename source directories like `cmd/picoclaw/` if desired after the module/import path decision.
- Sweep purely internal symbol names and test fixtures that still use `picoclaw` once the public/runtime rename is stable.

## Self-Review

- Spec coverage:
  - identity/name leftovers -> Tasks 3-5
  - top-level tree cleanup -> Task 1
  - build/runtime/operator dependency ordering -> Tasks 3-5
  - docs/history separation -> Tasks 1-2
- Placeholder scan:
  - no `TODO`/`TBD` placeholders
  - deferred module-path rename is explicit and justified by missing remote slug
- Type consistency:
  - `.planning/` is the internal bucket
  - `docs/history/` is the in-repo historical docs bucket
  - `reference/` is for non-product reference payloads
  - `support/codex-account-manager/` is the companion tool bucket
