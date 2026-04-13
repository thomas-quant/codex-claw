package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentModelConfig_UnmarshalString(t *testing.T) {
	var m AgentModelConfig
	if err := json.Unmarshal([]byte(`"gpt-4"`), &m); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if m.Primary != "gpt-4" {
		t.Errorf("Primary = %q, want 'gpt-4'", m.Primary)
	}
	if m.Fallbacks != nil {
		t.Errorf("Fallbacks = %v, want nil", m.Fallbacks)
	}
}

func TestAgentModelConfig_UnmarshalObject(t *testing.T) {
	var m AgentModelConfig
	data := `{"primary": "claude-opus", "fallbacks": ["gpt-4o-mini", "haiku"]}`
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if m.Primary != "claude-opus" {
		t.Errorf("Primary = %q, want 'claude-opus'", m.Primary)
	}
	if len(m.Fallbacks) != 2 {
		t.Fatalf("Fallbacks len = %d, want 2", len(m.Fallbacks))
	}
	if m.Fallbacks[0] != "gpt-4o-mini" || m.Fallbacks[1] != "haiku" {
		t.Errorf("Fallbacks = %v", m.Fallbacks)
	}
}

func TestAgentModelConfig_MarshalString(t *testing.T) {
	m := AgentModelConfig{Primary: "gpt-4"}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `"gpt-4"` {
		t.Errorf("marshal = %s, want '\"gpt-4\"'", string(data))
	}
}

func TestAgentModelConfig_MarshalObject(t *testing.T) {
	m := AgentModelConfig{Primary: "claude-opus", Fallbacks: []string{"haiku"}}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]any
	json.Unmarshal(data, &result)
	if result["primary"] != "claude-opus" {
		t.Errorf("primary = %v", result["primary"])
	}
}

func TestAgentConfig_FullParse(t *testing.T) {
	jsonData := `{
		"agents": {
			"defaults": {
				"workspace": "~/.picoclaw/workspace",
				"model": "glm-4.7",
				"max_tokens": 8192,
				"max_tool_iterations": 20
			},
			"list": [
				{
					"id": "sales",
					"default": true,
					"name": "Sales Bot",
					"model": "gpt-4"
				},
				{
					"id": "support",
					"name": "Support Bot",
					"model": {
						"primary": "claude-opus",
						"fallbacks": ["haiku"]
					},
					"subagents": {
						"allow_agents": ["sales"]
					}
				}
			]
		},
		"bindings": [
			{
				"agent_id": "support",
				"match": {
					"channel": "telegram",
					"account_id": "*",
					"peer": {"kind": "direct", "id": "user123"}
				}
			}
		],
		"session": {
			"dm_scope": "per-peer",
			"identity_links": {
				"john": ["telegram:123", "discord:john#1234"]
			}
		}
	}`

	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(jsonData), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(cfg.Agents.List) != 2 {
		t.Fatalf("agents.list len = %d, want 2", len(cfg.Agents.List))
	}

	sales := cfg.Agents.List[0]
	if sales.ID != "sales" || !sales.Default || sales.Name != "Sales Bot" {
		t.Errorf("sales = %+v", sales)
	}
	if sales.Model == nil || sales.Model.Primary != "gpt-4" {
		t.Errorf("sales.Model = %+v", sales.Model)
	}

	support := cfg.Agents.List[1]
	if support.ID != "support" || support.Name != "Support Bot" {
		t.Errorf("support = %+v", support)
	}
	if support.Model == nil || support.Model.Primary != "claude-opus" {
		t.Errorf("support.Model = %+v", support.Model)
	}
	if len(support.Model.Fallbacks) != 1 || support.Model.Fallbacks[0] != "haiku" {
		t.Errorf("support.Model.Fallbacks = %v", support.Model.Fallbacks)
	}
	if support.Subagents == nil || len(support.Subagents.AllowAgents) != 1 {
		t.Errorf("support.Subagents = %+v", support.Subagents)
	}

	if len(cfg.Bindings) != 1 {
		t.Fatalf("bindings len = %d, want 1", len(cfg.Bindings))
	}
	binding := cfg.Bindings[0]
	if binding.AgentID != "support" || binding.Match.Channel != "telegram" {
		t.Errorf("binding = %+v", binding)
	}
	if binding.Match.Peer == nil || binding.Match.Peer.Kind != "direct" || binding.Match.Peer.ID != "user123" {
		t.Errorf("binding.Match.Peer = %+v", binding.Match.Peer)
	}

	if cfg.Session.DMScope != "per-peer" {
		t.Errorf("Session.DMScope = %q", cfg.Session.DMScope)
	}
	if len(cfg.Session.IdentityLinks) != 1 {
		t.Errorf("Session.IdentityLinks = %v", cfg.Session.IdentityLinks)
	}
	links := cfg.Session.IdentityLinks["john"]
	if len(links) != 2 {
		t.Errorf("john links = %v", links)
	}
}

func TestDefaultConfig_MCPMaxInlineTextChars(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Tools.MCP.GetMaxInlineTextChars() != DefaultMCPMaxInlineTextChars {
		t.Fatalf(
			"DefaultConfig().Tools.MCP.GetMaxInlineTextChars() = %d, want %d",
			cfg.Tools.MCP.GetMaxInlineTextChars(),
			DefaultMCPMaxInlineTextChars,
		)
	}
}

func TestLoadConfig_MCPMaxInlineTextChars(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := `{
		"tools": {
			"mcp": {
				"enabled": true,
				"max_inline_text_chars": 2048
			}
		}
	}`
	if err := os.WriteFile(configPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(configPath): %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got := cfg.Tools.MCP.GetMaxInlineTextChars(); got != 2048 {
		t.Fatalf("cfg.Tools.MCP.GetMaxInlineTextChars() = %d, want 2048", got)
	}
}

func TestConfig_BackwardCompat_NoAgentsList(t *testing.T) {
	jsonData := `{
		"agents": {
			"defaults": {
				"workspace": "~/.picoclaw/workspace",
				"model": "glm-4.7",
				"max_tokens": 8192,
				"max_tool_iterations": 20
			}
		}
	}`

	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(jsonData), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(cfg.Agents.List) != 0 {
		t.Errorf("agents.list should be empty for backward compat, got %d", len(cfg.Agents.List))
	}
	if len(cfg.Bindings) != 0 {
		t.Errorf("bindings should be empty, got %d", len(cfg.Bindings))
	}
}

// TestDefaultConfig_HeartbeatEnabled verifies heartbeat is enabled by default
func TestDefaultConfig_HeartbeatEnabled(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Heartbeat.Enabled {
		t.Error("Heartbeat should be enabled by default")
	}
}

// TestDefaultConfig_WorkspacePath verifies workspace path is correctly set
func TestDefaultConfig_WorkspacePath(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Workspace == "" {
		t.Error("Workspace should not be empty")
	}
}

// TestDefaultConfig_MaxTokens verifies max tokens has default value
func TestDefaultConfig_MaxTokens(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.MaxTokens == 0 {
		t.Error("MaxTokens should not be zero")
	}
}

// TestDefaultConfig_MaxToolIterations verifies max tool iterations has default value
func TestDefaultConfig_MaxToolIterations(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.MaxToolIterations == 0 {
		t.Error("MaxToolIterations should not be zero")
	}
}

// TestDefaultConfig_Temperature verifies temperature has default value
func TestDefaultConfig_Temperature(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Temperature != nil {
		t.Error("Temperature should be nil when not provided")
	}
}

// TestDefaultConfig_Gateway verifies gateway defaults
func TestDefaultConfig_Gateway(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Gateway.Host != "127.0.0.1" {
		t.Error("Gateway host should have default value")
	}
	if cfg.Gateway.Port == 0 {
		t.Error("Gateway port should have default value")
	}
	if cfg.Gateway.HotReload {
		t.Error("Gateway hot reload should be disabled by default")
	}
}

// TestDefaultConfig_Channels verifies channels are disabled by default
func TestDefaultConfig_Channels(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Channels.Telegram.Enabled {
		t.Error("Telegram should be disabled by default")
	}
	if cfg.Channels.Discord.Enabled {
		t.Error("Discord should be disabled by default")
	}
	if cfg.Channels.Slack.Enabled {
		t.Error("Slack should be disabled by default")
	}
	if cfg.Channels.Matrix.Enabled {
		t.Error("Matrix should be disabled by default")
	}
}

// TestDefaultConfig_WebTools verifies web tools config
func TestDefaultConfig_WebTools(t *testing.T) {
	cfg := DefaultConfig()

	// Verify web tools defaults
	if cfg.Tools.Web.Brave.MaxResults != 5 {
		t.Error("Expected Brave MaxResults 5, got ", cfg.Tools.Web.Brave.MaxResults)
	}
	if len(cfg.Tools.Web.Brave.APIKeys) != 0 {
		t.Error("Brave API key should be empty by default")
	}
	if cfg.Tools.Web.DuckDuckGo.MaxResults != 5 {
		t.Error("Expected DuckDuckGo MaxResults 5, got ", cfg.Tools.Web.DuckDuckGo.MaxResults)
	}
}

func TestDefaultConfig_ReadFileMode(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Tools.ReadFile.EffectiveMode() != ReadFileModeBytes {
		t.Fatalf("expected default read_file mode %q, got %q", ReadFileModeBytes, cfg.Tools.ReadFile.EffectiveMode())
	}
}

func TestSaveConfig_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("config file has permission %04o, want 0600", perm)
	}
}

func TestSaveConfig_OmitsLegacyModelField(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if strings.Contains(string(data), `"model_name": ""`) {
		t.Fatalf("saved config should omit legacy model_name field, got: %s", string(data))
	}
}

func TestSaveConfig_PreservesDisabledTelegramPlaceholder(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	cfg.Channels.Telegram.Placeholder.Enabled = false

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(data), `"placeholder": {`) {
		t.Fatalf("saved config should include telegram placeholder config, got: %s", string(data))
	}
	if !strings.Contains(string(data), `"enabled": false`) {
		t.Fatalf("saved config should persist placeholder.enabled=false, got: %s", string(data))
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Channels.Telegram.Placeholder.Enabled {
		t.Fatal("telegram placeholder should remain disabled after SaveConfig/LoadConfig round-trip")
	}
}

func TestSaveConfig_RoundTripsNonModelSecurityConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := securityPath(configPath)

	cfg := DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = *NewSecureString("telegram-secret-token")
	cfg.Tools.Skills.Github.Token = *NewSecureString("github-secret-token")
	cfg.Tools.Web.Brave.APIKeys = SecureStrings{NewSecureString("brave-secret-key")}

	if err := SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	securityData, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", securityPath, err)
	}
	if !strings.Contains(string(securityData), "telegram-secret-token") {
		t.Fatalf(".security.yml should contain telegram token, got: %s", string(securityData))
	}
	if !strings.Contains(string(securityData), "github-secret-token") {
		t.Fatalf(".security.yml should contain github token, got: %s", string(securityData))
	}
	if !strings.Contains(string(securityData), "brave-secret-key") {
		t.Fatalf(".security.yml should contain brave key, got: %s", string(securityData))
	}

	reloaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if got := reloaded.Channels.Telegram.Token.String(); got != "telegram-secret-token" {
		t.Fatalf("Telegram.Token = %q, want %q", got, "telegram-secret-token")
	}
	if got := reloaded.Tools.Skills.Github.Token.String(); got != "github-secret-token" {
		t.Fatalf("Skills.Github.Token = %q, want %q", got, "github-secret-token")
	}
	if got := reloaded.Tools.Web.Brave.APIKey(); got != "brave-secret-key" {
		t.Fatalf("Web.Brave.APIKey() = %q, want %q", got, "brave-secret-key")
	}
}

// TestConfig_Complete verifies all config fields are set
func TestConfig_Complete(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Workspace == "" {
		t.Error("Workspace should not be empty")
	}
	if cfg.Agents.Defaults.Temperature != nil {
		t.Error("Temperature should be nil when not provided")
	}
	if cfg.Agents.Defaults.MaxTokens == 0 {
		t.Error("MaxTokens should not be zero")
	}
	if cfg.Agents.Defaults.MaxToolIterations == 0 {
		t.Error("MaxToolIterations should not be zero")
	}
	if cfg.Gateway.Host != "127.0.0.1" {
		t.Error("Gateway host should have default value")
	}
	if cfg.Gateway.Port == 0 {
		t.Error("Gateway port should have default value")
	}
	if !cfg.Heartbeat.Enabled {
		t.Error("Heartbeat should be enabled by default")
	}
	if !cfg.Tools.Exec.AllowRemote {
		t.Error("Exec.AllowRemote should be true by default")
	}
}

func TestDefaultConfig_WebPreferNativeEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.Web.PreferNative {
		t.Fatal("DefaultConfig().Tools.Web.PreferNative should be true")
	}
}

func TestDefaultConfig_ToolFeedbackDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agents.Defaults.ToolFeedback.Enabled {
		t.Fatal("DefaultConfig().Agents.Defaults.ToolFeedback.Enabled should be false")
	}
}

func TestLoadConfig_ToolFeedbackDefaultsFalseWhenUnset(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(
		configPath,
		[]byte(`{"version":1,"agents":{"defaults":{"workspace":"./workspace"}}}`),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Agents.Defaults.ToolFeedback.Enabled {
		t.Fatal("agents.defaults.tool_feedback.enabled should remain false when unset in config file")
	}
}

func TestLoadConfig_WebPreferNativeDefaultsTrueWhenUnset(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1,"tools":{"web":{"enabled":true}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if !cfg.Tools.Web.PreferNative {
		t.Fatal("PreferNative should remain true when unset in config file")
	}
}

func TestLoadConfig_WebPreferNativeCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"tools":{"web":{"prefer_native":false}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Tools.Web.PreferNative {
		t.Fatal("PreferNative should be false when disabled in config file")
	}
}

func TestDefaultConfig_ExecAllowRemoteEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.Exec.AllowRemote {
		t.Fatal("DefaultConfig().Tools.Exec.AllowRemote should be true")
	}
}

func TestDefaultConfig_FilterSensitiveDataEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.FilterSensitiveData {
		t.Fatal("DefaultConfig().Tools.FilterSensitiveData should be true")
	}
}

func TestDefaultConfig_FilterMinLength(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Tools.FilterMinLength != 8 {
		t.Fatalf("DefaultConfig().Tools.FilterMinLength = %d, want 8", cfg.Tools.FilterMinLength)
	}
}

func TestToolsConfig_GetFilterMinLength(t *testing.T) {
	tests := []struct {
		name     string
		minLen   int
		expected int
	}{
		{"zero returns default", 0, 8},
		{"negative returns default", -1, 8},
		{"positive returns value", 16, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsConfig{FilterMinLength: tt.minLen}
			if got := cfg.GetFilterMinLength(); got != tt.expected {
				t.Errorf("GetFilterMinLength() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig_CronAllowCommandEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.Cron.AllowCommand {
		t.Fatal("DefaultConfig().Tools.Cron.AllowCommand should be true")
	}
}

func TestDefaultConfig_HooksDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Hooks.Enabled {
		t.Fatal("DefaultConfig().Hooks.Enabled should be true")
	}
	if cfg.Hooks.Defaults.ObserverTimeoutMS != 500 {
		t.Fatalf("ObserverTimeoutMS = %d, want 500", cfg.Hooks.Defaults.ObserverTimeoutMS)
	}
	if cfg.Hooks.Defaults.InterceptorTimeoutMS != 5000 {
		t.Fatalf("InterceptorTimeoutMS = %d, want 5000", cfg.Hooks.Defaults.InterceptorTimeoutMS)
	}
	if cfg.Hooks.Defaults.ApprovalTimeoutMS != 60000 {
		t.Fatalf("ApprovalTimeoutMS = %d, want 60000", cfg.Hooks.Defaults.ApprovalTimeoutMS)
	}
}

func TestDefaultConfig_LogLevel(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Gateway.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want \"fatal\"", cfg.Gateway.LogLevel)
	}
}

func TestLoadConfig_ExecAllowRemoteDefaultsTrueWhenUnset(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1,"tools":{"exec":{"enable_deny_patterns":true}}}`),
		0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if !cfg.Tools.Exec.AllowRemote {
		t.Fatal("tools.exec.allow_remote should remain true when unset in config file")
	}
}

func TestLoadConfig_CronAllowCommandDefaultsTrueWhenUnset(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(
		configPath,
		[]byte(`{"version":1,"tools":{"cron":{"exec_timeout_minutes":5}}}`),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if !cfg.Tools.Cron.AllowCommand {
		t.Fatal("tools.cron.allow_command should remain true when unset in config file")
	}
}

func TestLoadConfig_WebToolsProxy(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{
  "agents": {"defaults":{"workspace":"./workspace","max_tokens":8192,"max_tool_iterations":20}},
  "tools": {"web":{"proxy":"http://127.0.0.1:7890"}}
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Tools.Web.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("Tools.Web.Proxy = %q, want %q", cfg.Tools.Web.Proxy, "http://127.0.0.1:7890")
	}
}

func TestLoadConfig_HooksProcessConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{
  "version": 1,
  "hooks": {
    "processes": {
      "review-gate": {
        "enabled": true,
        "transport": "stdio",
        "command": ["uvx", "picoclaw-hook-reviewer"],
        "dir": "/tmp/hooks",
        "env": {
          "HOOK_MODE": "rewrite"
        },
        "observe": ["turn_start", "turn_end"],
        "intercept": ["before_tool", "approve_tool"]
      }
    },
    "builtins": {
      "audit": {
        "enabled": true,
        "priority": 5,
        "config": {
          "label": "audit"
        }
      }
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	processCfg, ok := cfg.Hooks.Processes["review-gate"]
	if !ok {
		t.Fatal("expected review-gate process hook")
	}
	if !processCfg.Enabled {
		t.Fatal("expected review-gate process hook to be enabled")
	}
	if processCfg.Transport != "stdio" {
		t.Fatalf("Transport = %q, want stdio", processCfg.Transport)
	}
	if len(processCfg.Command) != 2 || processCfg.Command[0] != "uvx" {
		t.Fatalf("Command = %v", processCfg.Command)
	}
	if processCfg.Dir != "/tmp/hooks" {
		t.Fatalf("Dir = %q, want /tmp/hooks", processCfg.Dir)
	}
	if processCfg.Env["HOOK_MODE"] != "rewrite" {
		t.Fatalf("HOOK_MODE = %q, want rewrite", processCfg.Env["HOOK_MODE"])
	}
	if len(processCfg.Observe) != 2 || processCfg.Observe[1] != "turn_end" {
		t.Fatalf("Observe = %v", processCfg.Observe)
	}
	if len(processCfg.Intercept) != 2 || processCfg.Intercept[1] != "approve_tool" {
		t.Fatalf("Intercept = %v", processCfg.Intercept)
	}

	builtinCfg, ok := cfg.Hooks.Builtins["audit"]
	if !ok {
		t.Fatal("expected audit builtin hook")
	}
	if !builtinCfg.Enabled {
		t.Fatal("expected audit builtin hook to be enabled")
	}
	if builtinCfg.Priority != 5 {
		t.Fatalf("Priority = %d, want 5", builtinCfg.Priority)
	}
	if !strings.Contains(string(builtinCfg.Config), `"audit"`) {
		t.Fatalf("Config = %s", string(builtinCfg.Config))
	}
	if cfg.Hooks.Defaults.ApprovalTimeoutMS != 60000 {
		t.Fatalf("ApprovalTimeoutMS = %d, want 60000", cfg.Hooks.Defaults.ApprovalTimeoutMS)
	}
}

// TestDefaultConfig_DMScope verifies the default dm_scope value
// TestDefaultConfig_SummarizationThresholds verifies summarization defaults
func TestDefaultConfig_SummarizationThresholds(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.SummarizeMessageThreshold != 20 {
		t.Errorf("SummarizeMessageThreshold = %d, want 20", cfg.Agents.Defaults.SummarizeMessageThreshold)
	}
	if cfg.Agents.Defaults.SummarizeTokenPercent != 75 {
		t.Errorf("SummarizeTokenPercent = %d, want 75", cfg.Agents.Defaults.SummarizeTokenPercent)
	}
}

func TestDefaultConfig_DMScope(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Session.DMScope != "per-channel-peer" {
		t.Errorf("Session.DMScope = %q, want 'per-channel-peer'", cfg.Session.DMScope)
	}
}

func TestDefaultConfig_WorkspacePath_Default(t *testing.T) {
	t.Setenv("PICOCLAW_HOME", "")

	var fakeHome string
	if runtime.GOOS == "windows" {
		fakeHome = `C:\tmp\home`
		t.Setenv("USERPROFILE", fakeHome)
	} else {
		fakeHome = "/tmp/home"
		t.Setenv("HOME", fakeHome)
	}

	cfg := DefaultConfig()
	want := filepath.Join(fakeHome, ".picoclaw", "workspace")

	if cfg.Agents.Defaults.Workspace != want {
		t.Errorf("Default workspace path = %q, want %q", cfg.Agents.Defaults.Workspace, want)
	}
}

func TestDefaultConfig_WorkspacePath_WithPicoclawHome(t *testing.T) {
	t.Setenv("PICOCLAW_HOME", "/custom/picoclaw/home")

	cfg := DefaultConfig()
	want := filepath.Join("/custom/picoclaw/home", "workspace")

	if cfg.Agents.Defaults.Workspace != want {
		t.Errorf("Workspace path with PICOCLAW_HOME = %q, want %q", cfg.Agents.Defaults.Workspace, want)
	}
}

func TestDefaultConfig_IsolationEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Isolation.Enabled {
		t.Fatal("DefaultConfig().Isolation.Enabled should be false")
	}
}

func TestConfig_UnmarshalIsolation(t *testing.T) {
	cfg := DefaultConfig()
	raw := []byte(`{
		"isolation": {
			"enabled": false,
			"expose_paths": [
				{"source":"/src","target":"/dst","mode":"ro"}
			]
		}
	}`)
	if err := json.Unmarshal(raw, cfg); err != nil {
		t.Fatalf("json.Unmarshal isolation config: %v", err)
	}
	if cfg.Isolation.Enabled {
		t.Fatal("Isolation.Enabled should be false after unmarshal")
	}
	if len(cfg.Isolation.ExposePaths) != 1 {
		t.Fatalf("ExposePaths len = %d, want 1", len(cfg.Isolation.ExposePaths))
	}
	if got := cfg.Isolation.ExposePaths[0]; got.Source != "/src" || got.Target != "/dst" || got.Mode != "ro" {
		t.Fatalf("ExposePaths[0] = %+v, want source=/src target=/dst mode=ro", got)
	}
}

// TestFlexibleStringSlice_UnmarshalText tests UnmarshalText with various comma separators
func TestFlexibleStringSlice_UnmarshalText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "English commas only",
			input:    "123,456,789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Chinese commas only",
			input:    "123，456，789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Mixed English and Chinese commas",
			input:    "123,456，789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Single value",
			input:    "123",
			expected: []string{"123"},
		},
		{
			name:     "Values with whitespace",
			input:    " 123 , 456 , 789 ",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "Only commas - English",
			input:    ",,",
			expected: []string{},
		},
		{
			name:     "Only commas - Chinese",
			input:    "，，",
			expected: []string{},
		},
		{
			name:     "Mixed commas with empty parts",
			input:    "123,,456，，789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Complex mixed values",
			input:    "user1@example.com，user2@test.com, admin@domain.org",
			expected: []string{"user1@example.com", "user2@test.com", "admin@domain.org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleStringSlice
			err := f.UnmarshalText([]byte(tt.input))
			if err != nil {
				t.Fatalf("UnmarshalText(%q) error = %v", tt.input, err)
			}

			if tt.expected == nil {
				if f != nil {
					t.Errorf("UnmarshalText(%q) = %v, want nil", tt.input, f)
				}
				return
			}

			if len(f) != len(tt.expected) {
				t.Errorf("UnmarshalText(%q) length = %d, want %d", tt.input, len(f), len(tt.expected))
				return
			}

			for i, v := range tt.expected {
				if f[i] != v {
					t.Errorf("UnmarshalText(%q)[%d] = %q, want %q", tt.input, i, f[i], v)
				}
			}
		})
	}
}

// TestFlexibleStringSlice_UnmarshalText_EmptySliceConsistency tests nil vs empty slice behavior
func TestFlexibleStringSlice_UnmarshalText_EmptySliceConsistency(t *testing.T) {
	t.Run("Empty string returns nil", func(t *testing.T) {
		var f FlexibleStringSlice
		err := f.UnmarshalText([]byte(""))
		if err != nil {
			t.Fatalf("UnmarshalText error = %v", err)
		}
		if f != nil {
			t.Errorf("Empty string should return nil, got %v", f)
		}
	})

	t.Run("Commas only returns empty slice", func(t *testing.T) {
		var f FlexibleStringSlice
		err := f.UnmarshalText([]byte(",,,"))
		if err != nil {
			t.Fatalf("UnmarshalText error = %v", err)
		}
		if f == nil {
			t.Error("Commas only should return empty slice, not nil")
		}
		if len(f) != 0 {
			t.Errorf("Expected empty slice, got %v", f)
		}
	})
}

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single string",
			input:    `"Thinking..."`,
			expected: []string{"Thinking..."},
		},
		{
			name:     "single number",
			input:    `123`,
			expected: []string{"123"},
		},
		{
			name:     "string array",
			input:    `["Thinking...", "Still working..."]`,
			expected: []string{"Thinking...", "Still working..."},
		},
		{
			name:     "mixed array",
			input:    `["123", 456]`,
			expected: []string{"123", "456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleStringSlice
			if err := json.Unmarshal([]byte(tt.input), &f); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", tt.input, err)
			}
			if len(f) != len(tt.expected) {
				t.Fatalf("json.Unmarshal(%s) len = %d, want %d", tt.input, len(f), len(tt.expected))
			}
			for i, want := range tt.expected {
				if f[i] != want {
					t.Fatalf("json.Unmarshal(%s)[%d] = %q, want %q", tt.input, i, f[i], want)
				}
			}
		})
	}
}

func TestLoadConfig_TelegramPlaceholderTextAcceptsSingleString(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{
		"version": 1,
		"agents": { "defaults": { "workspace": "", "max_tokens": 0, "max_tool_iterations": 0 } },
		"bindings": [],
		"session": {},
		"channels": {
			"telegram": {
				"enabled": true,
				"bot_token": "",
				"allow_from": [],
				"placeholder": {
					"enabled": true,
					"text": "Thinking..."
				}
			}
		},
		"gateway": {},
		"tools": {},
		"heartbeat": {},
		"devices": {},
		"voice": {}
	}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := []string(cfg.Channels.Telegram.Placeholder.Text); len(got) != 1 || got[0] != "Thinking..." {
		t.Fatalf("placeholder.text = %#v, want [\"Thinking...\"]", got)
	}
}

func TestConfigParsesLogLevel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version":1,"gateway":{"log_level":"debug"}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Gateway.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want \"debug\"", cfg.Gateway.LogLevel)
	}
}

func TestConfigLogLevelEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version":1}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// When config omits log_level, the DefaultConfig value ("fatal") is preserved.
	if cfg.Gateway.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want \"fatal\"", cfg.Gateway.LogLevel)
	}
}

func TestResolveGatewayLogLevel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version":1,"gateway":{"log_level":"debug"}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if got := ResolveGatewayLogLevel(cfgPath); got != "debug" {
		t.Fatalf("ResolveGatewayLogLevel() = %q, want %q", got, "debug")
	}
}

func TestResolveGatewayLogLevel_UsesEnvOverrideAndNormalizesInvalid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version":1,"gateway":{"log_level":"debug"}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Setenv("PICOCLAW_LOG_LEVEL", "warning")
	if got := ResolveGatewayLogLevel(cfgPath); got != "warn" {
		t.Fatalf("ResolveGatewayLogLevel() with env override = %q, want %q", got, "warn")
	}

	t.Setenv("PICOCLAW_LOG_LEVEL", "garbage")
	if got := ResolveGatewayLogLevel(cfgPath); got != DefaultGatewayLogLevel {
		t.Fatalf("ResolveGatewayLogLevel() with invalid env override = %q, want %q", got, DefaultGatewayLogLevel)
	}
}

func TestFilterSensitiveData(t *testing.T) {
	cfg := &Config{
		Tools: ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     8,
			Web: WebToolsConfig{
				Brave: BraveConfig{APIKeys: SecureStrings{NewSecureString("brave-secret-12345")}},
			},
			Skills: SkillsToolsConfig{
				Github: SkillsGithubConfig{Token: *NewSecureString("github-secret-12345")},
			},
		},
		Channels: ChannelsConfig{
			Telegram: TelegramConfig{Token: *NewSecureString("telegram-secret-12345")},
		},
	}

	if got := cfg.FilterSensitiveData("sk-key"); got != "sk-key" {
		t.Errorf("short content should not be filtered: got %q", got)
	}

	content := "Telegram token telegram-secret-12345 and GitHub token github-secret-12345 and Brave brave-secret-12345"
	expected := "Telegram token [FILTERED] and GitHub token [FILTERED] and Brave [FILTERED]"
	if got := cfg.FilterSensitiveData(content); got != expected {
		t.Errorf("filtering failed: got %q, want %q", got, expected)
	}

	cfg.Tools.FilterSensitiveData = false
	if got := cfg.FilterSensitiveData(content); got != content {
		t.Errorf("disabled filtering: got %q, want original %q", got, content)
	}
}

func TestFilterSensitiveData_MultipleKeys(t *testing.T) {
	cfg := &Config{
		Tools: ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     8,
			Web: WebToolsConfig{
				Brave: BraveConfig{APIKeys: SecureStrings{NewSecureString("key-one"), NewSecureString("key-two")}},
			},
			Skills: SkillsToolsConfig{
				Github: SkillsGithubConfig{Token: *NewSecureString("key-three")},
			},
		},
	}

	content := "key-one and key-two and key-three should be filtered"
	expected := "[FILTERED] and [FILTERED] and [FILTERED] should be filtered"
	if got := cfg.FilterSensitiveData(content); got != expected {
		t.Errorf("multiple keys: got %q, want %q", got, expected)
	}
}

func TestFilterSensitiveData_AllTokenTypes(t *testing.T) {
	cfg := &Config{
		// Channel tokens
		Channels: ChannelsConfig{
			Telegram: TelegramConfig{Token: *NewSecureString("telegram-bot-token-abcdef")},
			Discord:  DiscordConfig{Token: *NewSecureString("discord-bot-token-xyz789")},
			Slack: SlackConfig{
				BotToken: *NewSecureString("xoxb-slack-bot-token"),
				AppToken: *NewSecureString("xapp-slack-app-token"),
			},
			Matrix: MatrixConfig{AccessToken: *NewSecureString("matrix-access-token-abc")},
			Feishu: FeishuConfig{
				AppSecret:  *NewSecureString("feishu-app-secret-123"),
				EncryptKey: *NewSecureString("feishu-encrypt-key"),
			},
			DingTalk: DingTalkConfig{ClientSecret: *NewSecureString("dingtalk-client-secret")},
			OneBot:   OneBotConfig{AccessToken: *NewSecureString("onebot-access-token")},
			WeCom:    WeComConfig{Secret: *NewSecureString("wecom-secret")},
			Pico:     PicoConfig{Token: *NewSecureString("pico-token-abc123")},
			IRC: IRCConfig{
				Password:         *NewSecureString("irc-password"),
				NickServPassword: *NewSecureString("nickserv-pass"),
				SASLPassword:     *NewSecureString("sasl-pass"),
			},
		},
		Tools: ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     8,
			// Web tool API keys
			Web: WebToolsConfig{
				Brave:       BraveConfig{APIKeys: SecureStrings{NewSecureString("brave-api-key")}},
				Tavily:      TavilyConfig{APIKeys: SecureStrings{NewSecureString("tavily-api-key")}},
				Perplexity:  PerplexityConfig{APIKeys: SecureStrings{NewSecureString("perplexity-api-key")}},
				GLMSearch:   GLMSearchConfig{APIKey: *NewSecureString("glm-search-key")},
				BaiduSearch: BaiduSearchConfig{APIKey: *NewSecureString("baidu-search-key")},
			},
			// Skills tokens
			Skills: SkillsToolsConfig{
				Github: SkillsGithubConfig{Token: *NewSecureString("github-token-xyz")},
				Registries: SkillsRegistriesConfig{
					ClawHub: ClawHubRegistryConfig{AuthToken: *NewSecureString("clawhub-auth-token")},
				},
			},
		},
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "telegram_token",
			content: "Telegram token: telegram-bot-token-abcdef",
			want:    "Telegram token: [FILTERED]",
		},
		{
			name:    "discord_token",
			content: "Discord token: discord-bot-token-xyz789",
			want:    "Discord token: [FILTERED]",
		},
		{
			name:    "slack_tokens",
			content: "Slack bot: xoxb-slack-bot-token, app: xapp-slack-app-token",
			want:    "Slack bot: [FILTERED], app: [FILTERED]",
		},
		{
			name:    "matrix_token",
			content: "Matrix access token: matrix-access-token-abc",
			want:    "Matrix access token: [FILTERED]",
		},
		{
			name:    "brave_api_key",
			content: "Brave key: brave-api-key",
			want:    "Brave key: [FILTERED]",
		},
		{
			name:    "tavily_api_key",
			content: "Tavily key: tavily-api-key",
			want:    "Tavily key: [FILTERED]",
		},
		{
			name:    "github_token",
			content: "GitHub token: github-token-xyz",
			want:    "GitHub token: [FILTERED]",
		},
		{
			name:    "irc_passwords",
			content: "IRC password: irc-password, nickserv: nickserv-pass",
			want:    "IRC password: [FILTERED], nickserv: [FILTERED]",
		},
		{
			name:    "mixed_content",
			content: "Telegram token telegram-bot-token-abcdef and GitHub token github-token-xyz",
			want:    "Telegram token [FILTERED] and GitHub token [FILTERED]",
		},
		{
			name:    "short_key_not_filtered",
			content: "Key abc not filtered because length < 8",
			want:    "Key abc not filtered because length < 8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.FilterSensitiveData(tt.content); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// makeBackup tests
// ---------------------------------------------------------------------------

// TestMakeBackup_WithDateSuffix verifies backup files include a date suffix.
func TestMakeBackup_WithDateSuffix(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := makeBackup(configPath); err != nil {
		t.Fatalf("makeBackup: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	var hasDatedBackup bool
	for _, e := range entries {
		if matched, _ := filepath.Match("config.json.20*.bak", e.Name()); matched {
			hasDatedBackup = true
			// Verify backup content matches original
			bakPath := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(bakPath)
			if err != nil {
				t.Fatalf("ReadFile backup: %v", err)
			}
			if string(data) != `{"version":2}` {
				t.Errorf("backup content = %q, want original content", string(data))
			}
			break
		}
	}
	if !hasDatedBackup {
		t.Error("expected backup file with date suffix pattern config.json.20*.bak")
	}
}

// TestMakeBackup_AlsoBacksSecurityFile verifies that the security config file
// is also backed up with the same date suffix.
func TestMakeBackup_AlsoBacksSecurityFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	secPath := securityPath(configPath)

	os.WriteFile(configPath, []byte(`{"version":2}`), 0o600)
	os.WriteFile(secPath, []byte("{}\n"), 0o600)

	if err := makeBackup(configPath); err != nil {
		t.Fatalf("makeBackup: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	configBackups := 0
	secBackups := 0
	for _, e := range entries {
		if matched, _ := filepath.Match("config.json.20*.bak", e.Name()); matched {
			configBackups++
		}
		if matched, _ := filepath.Match(".security.yml.20*.bak", e.Name()); matched {
			secBackups++
		}
	}
	if configBackups != 1 {
		t.Errorf("expected 1 config backup, got %d", configBackups)
	}
	if secBackups != 1 {
		t.Errorf("expected 1 security backup, got %d", secBackups)
	}
}

// TestMakeBackup_NonexistentFileSkipsBackup verifies that makeBackup returns nil
// when the config file does not exist (no error, no panic).
func TestMakeBackup_NonexistentFileSkipsBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nonexistent.json")

	if err := makeBackup(configPath); err != nil {
		t.Fatalf("makeBackup on nonexistent file should return nil, got: %v", err)
	}
}

// TestMakeBackup_OnlyConfigNoSecurity verifies backup succeeds when only
// the config file exists and no security file.
func TestMakeBackup_OnlyConfigNoSecurity(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(`{"version":2}`), 0o600)

	if err := makeBackup(configPath); err != nil {
		t.Fatalf("makeBackup: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	configBackups := 0
	secBackups := 0
	for _, e := range entries {
		if matched, _ := filepath.Match("config.json.20*.bak", e.Name()); matched {
			configBackups++
		}
		if matched, _ := filepath.Match(".security.yml.20*.bak", e.Name()); matched {
			secBackups++
		}
	}
	if configBackups != 1 {
		t.Errorf("expected 1 config backup, got %d", configBackups)
	}
	if secBackups != 0 {
		t.Errorf("expected 0 security backups when no security file exists, got %d", secBackups)
	}
}

// TestMakeBackup_SameDateSuffix verifies that config and security backups
// share the same date suffix (they are created in the same makeBackup call).
func TestMakeBackup_SameDateSuffix(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	secPath := securityPath(configPath)

	os.WriteFile(configPath, []byte(`{"version":2}`), 0o600)
	os.WriteFile(secPath, []byte(`key: value`), 0o600)

	if err := makeBackup(configPath); err != nil {
		t.Fatalf("makeBackup: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var configDate, secDate string
	for _, e := range entries {
		name := e.Name()
		// Extract date part: after the last . before .bak
		// e.g. config.json.20260330.bak → 20260330
		if strings.HasPrefix(name, "config.json.") && strings.HasSuffix(name, ".bak") {
			configDate = strings.TrimPrefix(name, "config.json.")
			configDate = strings.TrimSuffix(configDate, ".bak")
		}
		if strings.HasPrefix(name, ".security.yml.") && strings.HasSuffix(name, ".bak") {
			secDate = strings.TrimPrefix(name, ".security.yml.")
			secDate = strings.TrimSuffix(secDate, ".bak")
		}
	}
	if configDate == "" {
		t.Fatal("config backup file not found")
	}
	if secDate == "" {
		t.Fatal("security backup file not found")
	}
	if configDate != secDate {
		t.Errorf("config backup date = %q, security backup date = %q, should match", configDate, secDate)
	}
}
