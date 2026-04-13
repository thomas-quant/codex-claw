# Phase 7: Channel Surface Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the surviving fork surface consistently present only Telegram and Discord by cleaning English docs, examples, product-facing tests, and residual removed-channel wording without changing channel runtime behavior.

**Architecture:** Keep the generic channel manager and runtime wiring intact. Treat this as a narrow surface-consistency pass: clean the English-facing docs and examples first, then tighten product-facing tests and help text so the fork no longer implies support for removed channels.

**Tech Stack:** Markdown docs, JSON examples, Go tests, `go test`

---

## File Structure

### Modify

- `docs/chat-apps.md` — make the top-level chat-app doc fully Telegram/Discord-only
- `docs/channels/telegram/README.md` — remove any stale wording that implies a broader channel matrix
- `docs/channels/discord/README.md` — same cleanup for Discord
- `docs/configuration.md` — keep channel examples and wording aligned with Telegram/Discord-only support
- `docs/security_configuration.md` — ensure channel secret examples stay Telegram/Discord-only
- `docs/troubleshooting.md` — keep channel troubleshooting scoped to the surviving integrations
- `config/config.example.json` — ensure only Telegram/Discord remain in the channel example surface
- `pkg/channels/README.md` — keep the shared framework docs aligned with the fork boundary
- `pkg/channels/manager_runtime_test.go` — remove removed-channel names from product-facing runtime assertions
- `cmd/picoclaw/internal/gateway/command.go` and/or other CLI/help text only if stale removed-channel wording is still present after the doc/test sweep

### Keep As-Is

- `pkg/channels` shared runtime/framework files
- `pkg/gateway/gateway.go` runtime imports for Telegram and Discord
- Telegram/Discord implementation behavior

## Task 1: Clean The English Channel Docs And Examples

**Files:**
- Modify: `docs/chat-apps.md`
- Modify: `docs/channels/telegram/README.md`
- Modify: `docs/channels/discord/README.md`
- Modify: `docs/configuration.md`
- Modify: `docs/security_configuration.md`
- Modify: `docs/troubleshooting.md`
- Modify: `config/config.example.json`
- Modify: `pkg/channels/README.md`

- [ ] **Step 1: Write a failing content checklist for the doc surface**

Create a scratch checklist in your notes and verify the current files still contain at least one of these stale patterns before editing:

```text
- removed channel names presented as supported integrations
- channel example snippets containing anything beyond telegram/discord
- wording that implies a broader shipped chat-channel matrix
- troubleshooting/help text that references removed channels as live product surface
```

Use these search commands to confirm the current red state:

```bash
rg -n 'matrix|whatsapp|slack|qq|wecom|feishu|irc|line|onebot|dingtalk' \
  docs/chat-apps.md docs/channels/telegram/README.md docs/channels/discord/README.md \
  docs/configuration.md docs/security_configuration.md docs/troubleshooting.md \
  config/config.example.json pkg/channels/README.md
```

Expected: one or more matches in the targeted English-facing surface, or a clear confirmation that only a subset of these files still need edits.

- [ ] **Step 2: Rewrite the docs/examples to the Telegram/Discord-only boundary**

Apply the following cleanup rules:

`docs/chat-apps.md`

```md
# Chat Apps Configuration

This fork keeps two chat surfaces:

- [Telegram](channels/telegram/README.md)
- [Discord](channels/discord/README.md)

Both run through the shared channel manager and start with:

```bash
picoclaw gateway
```
```

`pkg/channels/README.md`

```md
# Channel System

`pkg/channels` contains the shared channel manager, capability interfaces, and the Telegram/Discord channel implementations that remain in this fork.

## Fork Boundary

- Only `telegram` and `discord` are registered and supported.
- The generic manager stays in place for routing, retries, allowlists, typing, placeholders, and outbound splitting.
- Channel families outside the retained Telegram/Discord boundary are out of scope here.
```

`config/config.example.json`

```json
{
  "channels": {
    "telegram": {
      "enabled": false
    },
    "discord": {
      "enabled": false
    }
  }
}
```

For the remaining English docs, remove any stale removed-channel wording and keep examples/snippets constrained to Telegram and Discord.

- [ ] **Step 3: Run the doc-surface search again**

Run:

```bash
rg -n 'matrix|whatsapp|slack|qq|wecom|feishu|irc|line|onebot|dingtalk' \
  docs/chat-apps.md docs/channels/telegram/README.md docs/channels/discord/README.md \
  docs/configuration.md docs/security_configuration.md docs/troubleshooting.md \
  config/config.example.json pkg/channels/README.md
```

Expected: no matches that still advertise removed channels in the targeted English-facing surface.

- [ ] **Step 4: Commit**

```bash
git add docs/chat-apps.md docs/channels/telegram/README.md docs/channels/discord/README.md
git add docs/configuration.md docs/security_configuration.md docs/troubleshooting.md
git add config/config.example.json pkg/channels/README.md
git commit -m "docs(channels): align fork surface to telegram and discord"
```

## Task 2: Clean Product-Facing Channel Tests And Runtime Assertions

**Files:**
- Modify: `pkg/channels/manager_runtime_test.go`
- Modify: any other touched test file only if it still uses removed real channel names in product-facing assertions

- [ ] **Step 1: Write failing test assertions for the narrowed runtime surface**

`pkg/channels/manager_runtime_test.go` should assert only the surviving runtime surface. Add or tighten assertions like:

```go
func TestNewManager_OnlyInitializesTelegramAndDiscord(t *testing.T) {
	// existing setup...

	got := m.GetEnabledChannels()
	want := map[string]bool{
		"telegram": true,
		"discord":  true,
	}
	if len(got) != len(want) {
		t.Fatalf("GetEnabledChannels() = %v, want %d channels", got, len(want))
	}
	for _, name := range got {
		if !want[name] {
			t.Fatalf("GetEnabledChannels() unexpectedly included %q", name)
		}
	}

	if _, ok := m.GetChannel("matrix"); ok {
		t.Fatal("expected matrix to be omitted from the runtime manager")
	}
	if _, ok := m.GetChannel("irc"); ok {
		t.Fatal("expected irc to be omitted from the runtime manager")
	}
}
```

If the file already has this shape, tighten any remaining assertions or helper text that still present removed channels as meaningful runtime fixtures.

- [ ] **Step 2: Run the focused channel-runtime tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/channels -run 'TestNewManager_OnlyInitializesTelegramAndDiscord' -count=1
```

Expected: PASS after the runtime-facing assertions reflect the narrowed surface.

- [ ] **Step 3: Sweep for product-facing removed-channel names in touched tests**

Run:

```bash
rg -n 'matrix|whatsapp|slack|qq|wecom|feishu|irc|line|onebot|dingtalk' \
  pkg/channels pkg/agent pkg/audio cmd/picoclaw -g '*_test.go'
```

Only rewrite matches that are product-facing examples, snapshots, or assertions. Leave purely generic/internal synthetic channel fixtures alone if the channel identity is irrelevant.

- [ ] **Step 4: Commit**

```bash
git add pkg/channels/manager_runtime_test.go
git commit -m "test(channels): tighten surviving runtime surface assertions"
```

## Task 3: Clean Residual User-Facing Removed-Channel Text And Verify

**Files:**
- Modify: `cmd/picoclaw/internal/gateway/command.go` only if needed
- Modify: any small residual help-text or comment file only if it still presents removed channels as shipped product surface

- [ ] **Step 1: Search the remaining English/runtime-facing text for stale removed-channel references**

Run:

```bash
rg -n 'matrix|whatsapp|slack|qq|wecom|feishu|irc|line|onebot|dingtalk' \
  cmd/picoclaw docs pkg/channels config \
  -g '*.go' -g '*.md' -g '*.json'
```

Expected: only intentional historical references, already-cleaned docs, or non-English files outside this phase. Identify any remaining English/product-facing text that still misrepresents the fork.

- [ ] **Step 2: Apply the minimal residual cleanup**

If you find stale user-facing text, rewrite it directly. Keep the change minimal. Example shape:

```go
// Before
"Supports Telegram, Discord, Slack, and Matrix bots"

// After
"Supports Telegram and Discord bots"
```

Do not refactor runtime code in this task.

- [ ] **Step 3: Run the narrow verification suite**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/channels ./cmd/picoclaw -count=1
```

Expected: PASS

- [ ] **Step 4: Manually review the final diff for surface consistency**

Check that:

```text
- English docs/examples present only Telegram and Discord as shipped channels
- product-facing tests no longer present removed channels as supported runtime surface
- no new runtime behavior was introduced
```

- [ ] **Step 5: Commit**

```bash
git add cmd/picoclaw/internal/gateway/command.go docs config pkg/channels
git commit -m "chore(channels): remove stale removed-channel surface"
```

## Self-Review

- Spec coverage: the plan covers English docs/examples, product-facing tests, and residual removed-channel text while explicitly leaving the runtime architecture alone.
- Placeholder scan: no `TODO`/`TBD` placeholders remain.
- Type consistency: the plan stays within existing file boundaries and does not invent new runtime APIs.
