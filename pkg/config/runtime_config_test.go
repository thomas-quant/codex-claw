package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_RuntimeBlockOnly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := `{
  "runtime": {
    "codex": {
      "default_model": "gpt-5.4",
      "default_thinking": "high",
      "fast": true,
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
  }
}`
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if got := cfg.Runtime.Codex.DefaultModel; got != "gpt-5.4" {
		t.Fatalf("cfg.Runtime.Codex.DefaultModel = %q, want %q", got, "gpt-5.4")
	}
	if got := cfg.Runtime.Codex.DefaultThinking; got != "high" {
		t.Fatalf("cfg.Runtime.Codex.DefaultThinking = %q, want %q", got, "high")
	}
	if !cfg.Runtime.Codex.Fast {
		t.Fatal("cfg.Runtime.Codex.Fast = false, want true")
	}
	if got := cfg.Runtime.Codex.AutoCompactThresholdPercent; got != 30 {
		t.Fatalf("cfg.Runtime.Codex.AutoCompactThresholdPercent = %d, want 30", got)
	}
	if len(cfg.Runtime.Codex.DiscoveryFallbackModels) != 2 {
		t.Fatalf(
			"cfg.Runtime.Codex.DiscoveryFallbackModels len = %d, want 2",
			len(cfg.Runtime.Codex.DiscoveryFallbackModels),
		)
	}
	if !cfg.Runtime.Fallback.DeepSeek.Enabled {
		t.Fatal("cfg.Runtime.Fallback.DeepSeek.Enabled = false, want true")
	}
	if got := cfg.Runtime.Fallback.DeepSeek.Model; got != "deepseek-chat" {
		t.Fatalf("cfg.Runtime.Fallback.DeepSeek.Model = %q, want %q", got, "deepseek-chat")
	}
	if got := cfg.Runtime.Fallback.DeepSeek.APIBase; got != "https://api.deepseek.com/v1" {
		t.Fatalf("cfg.Runtime.Fallback.DeepSeek.APIBase = %q, want %q", got, "https://api.deepseek.com/v1")
	}
}

func TestLoadConfig_RuntimeSandboxBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := `{
  "runtime": {
    "codex": {
      "default_model": "gpt-5.4",
      "sandbox_mode": "workspace-write",
      "workspace_write": {
        "writable_roots": ["/workspace", "/tmp"],
        "network_access": true
      }
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if got := cfg.Runtime.Codex.SandboxMode; got != "workspace-write" {
		t.Fatalf("cfg.Runtime.Codex.SandboxMode = %q, want %q", got, "workspace-write")
	}
	if len(cfg.Runtime.Codex.WorkspaceWrite.WritableRoots) != 2 {
		t.Fatalf(
			"cfg.Runtime.Codex.WorkspaceWrite.WritableRoots len = %d, want 2",
			len(cfg.Runtime.Codex.WorkspaceWrite.WritableRoots),
		)
	}
	if got := cfg.Runtime.Codex.WorkspaceWrite.WritableRoots[0]; got != "/workspace" {
		t.Fatalf("cfg.Runtime.Codex.WorkspaceWrite.WritableRoots[0] = %q, want %q", got, "/workspace")
	}
	if got := cfg.Runtime.Codex.WorkspaceWrite.WritableRoots[1]; got != "/tmp" {
		t.Fatalf("cfg.Runtime.Codex.WorkspaceWrite.WritableRoots[1] = %q, want %q", got, "/tmp")
	}
	if !cfg.Runtime.Codex.WorkspaceWrite.NetworkAccess {
		t.Fatal("cfg.Runtime.Codex.WorkspaceWrite.NetworkAccess = false, want true")
	}
}

func TestLoadConfig_RejectsLegacyProviderKeys(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := `{
  "agents": {
    "defaults": {
      "workspace": "~/.codex-claw/workspace",
      "provider": "deepseek",
      "model_name": "deepseek-chat",
      "model": "deepseek-chat"
    }
  },
  "providers": {
    "deepseek": {
      "model": "deepseek/deepseek-chat"
    }
  },
  "model_list": [
    {
      "model_name": "deepseek-chat",
      "model": "deepseek/deepseek-chat"
    }
  ]
}`
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want legacy config rejection")
	}
	if got := err.Error(); !strings.Contains(got, "legacy model/provider config is no longer supported") {
		t.Fatalf("LoadConfig() error = %q, want legacy rejection message", got)
	}
}
