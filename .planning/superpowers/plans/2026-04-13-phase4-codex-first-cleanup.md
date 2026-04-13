# Phase 4 Codex-First Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace PicoClaw's legacy provider/auth/launcher surface with a hard-broken Codex-first runtime shape while preserving the agent, tool, MCP, cron, and channel backbone.

**Architecture:** Keep the existing agent/session/tool runtime intact and narrow the model side of the system to `runtime.codex` plus `runtime.fallback.deepseek`. Treat this as one destructive cleanup patch: first lock the new config contract with tests, then rewrite provider selection, then remove dead CLI/channel/top-level surfaces, and finally repair docs, dependencies, and verification around the smaller product boundary.

**Tech Stack:** Go 1.25, Cobra CLI, existing PicoClaw agent/session/tool stack, Codex app-server provider, DeepSeek HTTP fallback, Go tests, Makefile

---

## File Structure

- `pkg/config/config.go`
  New top-level `RuntimeConfig` types and hard-fail rejection of legacy provider/model/auth keys.
- `pkg/config/defaults.go`
  Default `runtime.codex` and `runtime.fallback.deepseek` values.
- `pkg/config/runtime_config_test.go`
  New schema tests for the Codex-first config contract.
- `pkg/providers/legacy_provider.go`
  Rewrite `CreateProvider(cfg *config.Config)` to stop reading `model_list` and create only the Codex provider.
- `pkg/providers/factory_provider.go`
  Delete old provider switch branches and keep only Codex app-server plus DeepSeek fallback creation helpers.
- `pkg/providers/codex_first_factory_test.go`
  Tests for Codex primary and DeepSeek fallback creation.
- `pkg/commands/builtin.go`
  Remove or rewrite built-in commands that still describe the old provider catalog.
- `pkg/commands/cmd_switch.go`
  Convert `/switch model to ...` into a thin alias for the Codex runtime `/set model ...` path.
- `pkg/commands/cmd_show.go`
  Convert `/show model` to Codex runtime status output.
- `pkg/commands/cmd_list.go`
  Convert `/list models` to Codex runtime discovery output.
- `pkg/commands/runtime_aliases_test.go`
  Tests for the rewritten model/status command aliases.
- `cmd/picoclaw/main.go`
  Remove legacy root commands for auth, model, and launcher-era surfaces.
- `cmd/picoclaw/main_test.go`
  Assert the root command no longer exposes auth/model commands.
- `pkg/channels/manager.go`
  Keep the generic manager but register only Telegram and Discord.
- `pkg/channels/manager_runtime_test.go`
  Assert only the surviving channel factories are wired in.
- `Makefile`
  Remove launcher/web/WhatsApp-native targets and keep the fork-relevant build/test targets.
- `config/config.example.json`
  Replace the old schema example with the new hard-broken `runtime` block.
- `go.mod`
  Drop launcher/auth/provider/channel dependencies no longer referenced after the cleanup.
- `go.sum`
  Regenerated dependency lockfile after `go mod tidy`.

## Task 1: Lock the new config contract

**Files:**
- Create: `pkg/config/runtime_config_test.go`
- Modify: `pkg/config/config.go`
- Modify: `pkg/config/defaults.go`
- Modify: `config/config.example.json`
- Delete: `pkg/config/config_old.go`
- Delete: `pkg/config/migration.go`
- Delete: `pkg/config/migration_test.go`
- Delete: `pkg/config/migration_integration_test.go`
- Delete: `pkg/config/multikey_test.go`

- [ ] **Step 1: Write failing config tests for the new runtime block**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig_RuntimeBlockOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{
	  "runtime": {
	    "codex": {
	      "default_model": "gpt-5.4",
	      "default_thinking": "high",
	      "fast": false,
	      "auto_compact_threshold_percent": 30,
	      "discovery_fallback_models": ["gpt-5.4", "gpt-5.4-mini"]
	    },
	    "fallback": {
	      "deepseek": {
	        "enabled": true,
	        "model": "deepseek-chat",
	        "api_base": "https://api.deepseek.com/v1"
	      }
	    }
	  },
	  "agents": {
	    "defaults": {
	      "workspace": "/tmp/workspace"
	    }
	  },
	  "channels": {
	    "telegram": { "enabled": false },
	    "discord": { "enabled": false }
	  }
	}`
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", cfg.Runtime.Codex.DefaultModel)
	require.Equal(t, "high", cfg.Runtime.Codex.DefaultThinking)
	require.Equal(t, 30, cfg.Runtime.Codex.AutoCompactThresholdPercent)
	require.Equal(t, "deepseek-chat", cfg.Runtime.Fallback.DeepSeek.Model)
}

func TestLoadConfig_RejectsLegacyProviderKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{
	  "agents": {
	    "defaults": {
	      "provider": "openai",
	      "model_name": "gpt-5.4"
	    }
	  },
	  "model_list": [
	    { "model_name": "gpt-5.4", "model": "openai/gpt-5.4" }
	  ]
	}`
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	_, err := LoadConfig(path)
	require.ErrorContains(t, err, "legacy model/provider config is no longer supported")
}
```

- [ ] **Step 2: Run the config tests to verify they fail**

Run: `go test ./pkg/config -run 'TestLoadConfig_(RuntimeBlockOnly|RejectsLegacyProviderKeys)' -count=1`

Expected: FAIL because `Config` does not have a `Runtime` block yet and legacy keys still load through compatibility code.

- [ ] **Step 3: Implement the hard-broken runtime schema**

```go
type RuntimeConfig struct {
	Codex    CodexRuntimeConfig    `json:"codex"`
	Fallback FallbackRuntimeConfig `json:"fallback,omitempty"`
}

type CodexRuntimeConfig struct {
	DefaultModel               string   `json:"default_model"`
	DefaultThinking            string   `json:"default_thinking,omitempty"`
	Fast                       bool     `json:"fast,omitempty"`
	AutoCompactThresholdPercent int     `json:"auto_compact_threshold_percent,omitempty"`
	DiscoveryFallbackModels    []string `json:"discovery_fallback_models,omitempty"`
}

type FallbackRuntimeConfig struct {
	DeepSeek DeepSeekFallbackConfig `json:"deepseek,omitempty"`
}

type DeepSeekFallbackConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Model   string `json:"model,omitempty"`
	APIBase string `json:"api_base,omitempty"`
}

type Config struct {
	Version   int            `json:"version" yaml:"-"`
	Runtime   RuntimeConfig  `json:"runtime" yaml:"runtime"`
	Isolation IsolationConfig `json:"isolation,omitempty" yaml:"-"`
	Agents    AgentsConfig   `json:"agents" yaml:"-"`
	Bindings  []AgentBinding `json:"bindings,omitempty" yaml:"-"`
	Session   SessionConfig  `json:"session,omitempty" yaml:"-"`
	Channels  ChannelsConfig `json:"channels" yaml:"channels"`
	Gateway   GatewayConfig  `json:"gateway" yaml:"-"`
	Hooks     HooksConfig    `json:"hooks,omitempty" yaml:"-"`
	Tools     ToolsConfig    `json:"tools" yaml:",inline"`
	Heartbeat HeartbeatConfig `json:"heartbeat" yaml:"-"`
	Devices   DevicesConfig  `json:"devices" yaml:"-"`
	Voice     VoiceConfig    `json:"voice" yaml:"-"`
	BuildInfo BuildInfo      `json:"build_info,omitempty" yaml:"-"`
}

func rejectLegacyRuntimeKeys(raw []byte) error {
	if bytes.Contains(raw, []byte(`"model_list"`)) || bytes.Contains(raw, []byte(`"provider"`)) {
		return fmt.Errorf("legacy model/provider config is no longer supported")
	}
	return nil
}
```

Also update `DefaultConfig()` so the new example defaults are:

```go
Runtime: RuntimeConfig{
	Codex: CodexRuntimeConfig{
		DefaultModel:                "gpt-5.4",
		DefaultThinking:             "medium",
		AutoCompactThresholdPercent: 30,
		DiscoveryFallbackModels:     []string{"gpt-5.4", "gpt-5.4-mini"},
	},
	Fallback: FallbackRuntimeConfig{
		DeepSeek: DeepSeekFallbackConfig{
			Enabled: true,
			Model:   "deepseek-chat",
			APIBase: "https://api.deepseek.com/v1",
		},
	},
},
```

- [ ] **Step 4: Run the package tests and verify the new config example loads**

Run: `go test ./pkg/config -count=1`

Expected: PASS for the surviving config tests, with migration-era tests removed from the package.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/config.go pkg/config/defaults.go pkg/config/runtime_config_test.go config/config.example.json
git add -u pkg/config
git commit -m "refactor(config): replace legacy provider schema with runtime block"
```

## Task 2: Collapse provider creation to Codex + DeepSeek

**Files:**
- Create: `pkg/providers/codex_first_factory_test.go`
- Modify: `pkg/providers/legacy_provider.go`
- Modify: `pkg/providers/factory_provider.go`
- Modify: `pkg/providers/fallback.go`
- Modify: `pkg/providers/factory_provider_test.go`
- Delete: `pkg/providers/antigravity_provider.go`
- Delete: `pkg/providers/antigravity_provider_test.go`
- Delete: `pkg/providers/claude_cli_provider.go`
- Delete: `pkg/providers/claude_cli_provider_integration_test.go`
- Delete: `pkg/providers/claude_cli_provider_test.go`
- Delete: `pkg/providers/claude_provider.go`
- Delete: `pkg/providers/claude_provider_test.go`
- Delete: `pkg/providers/codex_cli_credentials.go`
- Delete: `pkg/providers/codex_cli_credentials_test.go`
- Delete: `pkg/providers/codex_cli_provider.go`
- Delete: `pkg/providers/codex_cli_provider_integration_test.go`
- Delete: `pkg/providers/codex_cli_provider_test.go`
- Delete: `pkg/providers/codex_provider.go`
- Delete: `pkg/providers/codex_provider_test.go`
- Delete: `pkg/providers/gemini_provider.go`
- Delete: `pkg/providers/gemini_provider_test.go`
- Delete: `pkg/providers/github_copilot_provider.go`
- Delete: `pkg/providers/anthropic/`
- Delete: `pkg/providers/anthropic_messages/`
- Delete: `pkg/providers/azure/`
- Delete: `pkg/providers/bedrock/`

- [ ] **Step 1: Write failing provider tests for the narrowed runtime**

```go
package providers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestCreateProvider_UsesCodexRuntimeDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runtime.Codex.DefaultModel = "gpt-5.4-mini"
	cfg.Agents.Defaults.ModelName = ""

	provider, modelID, err := CreateProvider(cfg)
	require.NoError(t, err)
	require.IsType(t, &CodexAppServerProvider{}, provider)
	require.Equal(t, "gpt-5.4-mini", modelID)
}

func TestCreateDeepSeekFallbackCandidate_UsesRuntimeBlock(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-chat"

	candidates := RuntimeFallbackCandidates(cfg)
	require.Len(t, candidates, 1)
	require.Equal(t, "deepseek", candidates[0].Provider)
	require.Equal(t, "deepseek-chat", candidates[0].Model)
}
```

- [ ] **Step 2: Run the provider tests to verify they fail**

Run: `go test ./pkg/providers -run 'TestCreate(Provider_UsesCodexRuntimeDefaults|DeepSeekFallbackCandidate_UsesRuntimeBlock)' -count=1`

Expected: FAIL because `CreateProvider` still depends on `model_list` and there is no runtime-derived fallback candidate helper.

- [ ] **Step 3: Rewrite the surviving provider path**

```go
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	model := strings.TrimSpace(cfg.Agents.Defaults.GetModelName())
	if model == "" {
		model = strings.TrimSpace(cfg.Runtime.Codex.DefaultModel)
	}
	if model == "" {
		return nil, "", fmt.Errorf("runtime.codex.default_model is required")
	}

	workspace := cfg.WorkspacePath()
	timeoutSeconds := cfg.Gateway.RequestTimeoutSeconds
	return NewCodexAppServerProvider(newCodexAppServerRunner(workspace, timeoutSeconds)), model, nil
}

func RuntimeFallbackCandidates(cfg *config.Config) []FallbackCandidate {
	if !cfg.Runtime.Fallback.DeepSeek.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Runtime.Fallback.DeepSeek.Model) == "" {
		return nil
	}
	return []FallbackCandidate{{
		Provider: "deepseek",
		Model:    cfg.Runtime.Fallback.DeepSeek.Model,
	}}
}
```

In `factory_provider.go`, keep only the surviving creation branches:

```go
switch provider {
case "codex":
	return NewCodexAppServerProvider(newCodexAppServerRunner(workspace, requestTimeoutSeconds)), model, nil
case "deepseek":
	return NewHTTPProvider(os.Getenv("DEEPSEEK_API_KEY"), cfg.Runtime.Fallback.DeepSeek.APIBase, ""), model, nil
default:
	return nil, fmt.Errorf("unsupported provider %q in codex-first fork", provider)
}
```

- [ ] **Step 4: Run the provider package tests**

Run: `go test ./pkg/providers -count=1`

Expected: PASS with only Codex/DeepSeek-related tests remaining in the package.

- [ ] **Step 5: Commit**

```bash
git add pkg/providers/legacy_provider.go pkg/providers/factory_provider.go pkg/providers/fallback.go
git add pkg/providers/codex_first_factory_test.go pkg/providers/factory_provider_test.go
git add -u pkg/providers
git commit -m "refactor(providers): collapse runtime to codex and deepseek"
```

## Task 3: Remove legacy CLI, auth, and model surfaces

**Files:**
- Modify: `cmd/picoclaw/main.go`
- Modify: `cmd/picoclaw/main_test.go`
- Modify: `pkg/commands/builtin.go`
- Modify: `pkg/commands/cmd_switch.go`
- Modify: `pkg/commands/cmd_show.go`
- Modify: `pkg/commands/cmd_list.go`
- Create: `pkg/commands/runtime_aliases_test.go`
- Delete: `cmd/picoclaw/internal/auth/`
- Delete: `cmd/picoclaw/internal/model/`

- [ ] **Step 1: Write failing tests for the surviving root and chat command surface**

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPicoclawCommand_DropsAuthAndModelCommands(t *testing.T) {
	cmd := NewPicoclawCommand()
	names := make([]string, 0, len(cmd.Commands()))
	for _, child := range cmd.Commands() {
		names = append(names, child.Name())
	}
	require.NotContains(t, names, "auth")
	require.NotContains(t, names, "model")
}
```

```go
package commands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSwitchModelAlias_UsesRuntimeController(t *testing.T) {
	var switchedTo string
	rt := &Runtime{
		SetModel: func(value string) (string, error) {
			switchedTo = value
			return "gpt-5.4", nil
		},
	}
	req := Request{Text: "/switch model to gpt-5.4-mini", Reply: func(string) error { return nil }}

	err := switchCommand().Handler(context.Background(), req, rt)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4-mini", switchedTo)
}
```

- [ ] **Step 2: Run the targeted command tests to verify they fail**

Run: `go test ./cmd/picoclaw ./pkg/commands -run 'Test(NewPicoclawCommand_DropsAuthAndModelCommands|SwitchModelAlias_UsesRuntimeController)' -count=1`

Expected: FAIL because the root CLI still wires auth/model commands and the alias commands still read legacy provider/config state.

- [ ] **Step 3: Rewrite the root command and runtime aliases**

Update the root command registration to:

```go
cmd.AddCommand(
	onboard.NewOnboardCommand(),
	agent.NewAgentCommand(),
	gateway.NewGatewayCommand(),
	status.NewStatusCommand(),
	cron.NewCronCommand(),
	migrate.NewMigrateCommand(),
	skills.NewSkillsCommand(),
	updater.NewUpdateCommand("picoclaw"),
	version.NewVersionCommand(),
)
```

Update the chat command aliases so they defer to the runtime controller instead of `model_list`:

```go
func handleSwitchModel(_ context.Context, req Request, rt *Runtime) error {
	target := strings.TrimSpace(strings.TrimPrefix(req.Text, "/switch model to "))
	if target == "" {
		return req.Reply("usage: /switch model to <model>")
	}
	oldModel, err := rt.SetModel(target)
	if err != nil {
		return err
	}
	return req.Reply(fmt.Sprintf("Model switched from %s to %s", oldModel, target))
}

func showModelHandler(_ context.Context, req Request, rt *Runtime) error {
	status := rt.ReadStatus()
	return req.Reply(fmt.Sprintf("model: %s", status.Model))
}

func listModelsHandler(_ context.Context, req Request, rt *Runtime) error {
	models := rt.ListModels()
	return req.Reply(strings.Join(modelNames(models), "\n"))
}
```

- [ ] **Step 4: Run the CLI and command tests**

Run: `go test ./cmd/picoclaw ./pkg/commands -count=1`

Expected: PASS with auth/model root commands removed and the remaining model-related chat commands backed by the runtime controller.

- [ ] **Step 5: Commit**

```bash
git add cmd/picoclaw/main.go cmd/picoclaw/main_test.go
git add pkg/commands/builtin.go pkg/commands/cmd_switch.go pkg/commands/cmd_show.go pkg/commands/cmd_list.go pkg/commands/runtime_aliases_test.go
git add -u cmd/picoclaw/internal/auth cmd/picoclaw/internal/model
git commit -m "refactor(cli): remove legacy auth and model surfaces"
```

## Task 4: Slim channels and delete launcher/web surfaces

**Files:**
- Modify: `pkg/channels/manager.go`
- Modify: `pkg/channels/manager_channel.go`
- Create: `pkg/channels/manager_runtime_test.go`
- Modify: `Makefile`
- Delete: `pkg/channels/irc/`
- Delete: `pkg/channels/pico/`
- Delete: `cmd/picoclaw-launcher-tui/`
- Delete: `web/`

- [ ] **Step 1: Write failing tests for the surviving channel registry**

```go
package channels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAvailableChannelKeys_RuntimeFork(t *testing.T) {
	keys := SupportedChannelKeys()
	require.ElementsMatch(t, []string{"discord", "telegram"}, keys)
}
```

- [ ] **Step 2: Run the channel tests to verify they fail**

Run: `go test ./pkg/channels -run TestAvailableChannelKeys_RuntimeFork -count=1`

Expected: FAIL because IRC/Pico are still registered or available from the manager registry.

- [ ] **Step 3: Rewrite registration and prune build targets**

Keep only the surviving channel registrations in the manager wiring:

```go
func SupportedChannelKeys() []string {
	return []string{"discord", "telegram"}
}
```

Reduce the `Makefile` target set to:

```make
.PHONY: all build clean help test vet lint fmt fix check

all: build

build: generate
	@GOARCH=${ARCH} $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_PATH)$(EXT) ./$(CMD_DIR)

test:
	@$(GO) test ./...
```

Then delete `pkg/channels/irc/`, `pkg/channels/pico/`, `cmd/picoclaw-launcher-tui/`, and `web/`.

- [ ] **Step 4: Run the channel and root build tests**

Run: `go test ./pkg/channels ./cmd/picoclaw -count=1`

Expected: PASS with only Telegram and Discord surviving in the channel layer and no launcher/TUI compile references left in the repo.

- [ ] **Step 5: Commit**

```bash
git add pkg/channels/manager.go pkg/channels/manager_channel.go pkg/channels/manager_runtime_test.go Makefile
git add -u pkg/channels/irc pkg/channels/pico cmd/picoclaw-launcher-tui web
git commit -m "refactor(runtime): remove launcher and unused channel surfaces"
```

## Task 5: Update docs, example config, and dependencies

**Files:**
- Modify: `config/config.example.json`
- Modify: `docs/configuration.md`
- Modify: `docs/providers.md`
- Modify: `docs/docker.md`
- Modify: `followups.md`
- Modify: `go.mod`
- Modify: `go.sum`
- Delete: launcher/auth/provider docs that no longer describe a surviving surface

- [ ] **Step 1: Write a failing verification script for the repo-level cleanup**

```bash
go test ./pkg/config ./pkg/providers ./pkg/channels ./pkg/commands ./pkg/agent -count=1
go test ./cmd/picoclaw -count=1
go mod tidy
git diff --exit-code go.mod go.sum
```

Expected: the first run should fail before this task because docs and dependency state still reference deleted surfaces.

- [ ] **Step 2: Rewrite docs and the example config to the surviving fork boundary**

Use this reduced example shape in `config/config.example.json`:

```json
{
  "runtime": {
    "codex": {
      "default_model": "gpt-5.4",
      "default_thinking": "medium",
      "fast": false,
      "auto_compact_threshold_percent": 30,
      "discovery_fallback_models": ["gpt-5.4", "gpt-5.4-mini"]
    },
    "fallback": {
      "deepseek": {
        "enabled": true,
        "model": "deepseek-chat",
        "api_base": "https://api.deepseek.com/v1"
      }
    }
  },
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace"
    }
  },
  "channels": {
    "telegram": { "enabled": false },
    "discord": { "enabled": false }
  }
}
```

Delete docs that are now false rather than trying to preserve compatibility text for launchers, OAuth, or removed providers.

- [ ] **Step 3: Tidy dependencies after the physical deletions**

Run:

```bash
go mod tidy
```

Then verify the removed dependency classes are actually gone from `go.mod`:

```bash
rg 'aws-sdk|mautrix|whatsmeow|bubbletea|vite|oauth' go.mod
```

Expected: no matches for deleted provider/channel/launcher dependency families.

- [ ] **Step 4: Run the narrowed verification suite**

Run:

```bash
go test ./pkg/config ./pkg/providers ./pkg/channels ./pkg/commands ./pkg/agent -count=1
go test ./cmd/picoclaw -count=1
```

Expected: PASS with the codex-first fork boundary compiling cleanly.

- [ ] **Step 5: Commit**

```bash
git add config/config.example.json docs/configuration.md docs/providers.md docs/docker.md followups.md go.mod go.sum
git add -u docs
git commit -m "chore: prune docs and dependencies for codex-first fork"
```

## Self-Review

- Spec coverage check:
  - hard-broken config rewrite: Task 1
  - Codex + DeepSeek provider collapse: Task 2
  - root CLI and runtime command cleanup: Task 3
  - launcher/web/channel deletion: Task 4
  - docs/build/dependency cleanup: Task 5
- Placeholder scan:
  - no `TODO`, `TBD`, or “implement later” markers remain
- Type consistency:
  - `RuntimeConfig`, `CodexRuntimeConfig`, `DeepSeekFallbackConfig`, and `RuntimeFallbackCandidates` are defined before later tasks rely on them
