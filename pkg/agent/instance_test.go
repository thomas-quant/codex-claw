package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/config"
	"github.com/thomas-quant/codex-claw/pkg/media"
	"github.com/thomas-quant/codex-claw/pkg/providers"
	"github.com/thomas-quant/codex-claw/pkg/tools"
)

func TestNewAgentInstance_UsesDefaultsTemperatureAndMaxTokens(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-instance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         1234,
				MaxToolIterations: 5,
			},
		},
	}

	configuredTemp := 1.0
	cfg.Agents.Defaults.Temperature = &configuredTemp

	provider := &mockProvider{}
	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, provider)

	if agent.MaxTokens != 1234 {
		t.Fatalf("MaxTokens = %d, want %d", agent.MaxTokens, 1234)
	}
	if agent.Temperature != 1.0 {
		t.Fatalf("Temperature = %f, want %f", agent.Temperature, 1.0)
	}
}

func TestNewAgentInstance_DefaultsMaxTokensWhenUnset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-instance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         0,
				MaxToolIterations: 5,
			},
		},
	}

	provider := &mockProvider{}
	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, provider)

	if agent.MaxTokens != 128000 {
		t.Fatalf("MaxTokens = %d, want %d", agent.MaxTokens, 128000)
	}
}

func TestNewAgentInstance_DefaultsTemperatureWhenZero(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-instance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         1234,
				MaxToolIterations: 5,
			},
		},
	}

	configuredTemp := 0.0
	cfg.Agents.Defaults.Temperature = &configuredTemp

	provider := &mockProvider{}
	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, provider)

	if agent.Temperature != 0.0 {
		t.Fatalf("Temperature = %f, want %f", agent.Temperature, 0.0)
	}
}

func TestNewAgentInstance_DefaultsTemperatureWhenUnset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-instance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         1234,
				MaxToolIterations: 5,
			},
		},
	}

	provider := &mockProvider{}
	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, provider)

	if agent.Temperature != 0.7 {
		t.Fatalf("Temperature = %f, want %f", agent.Temperature, 0.7)
	}
}

func TestNewAgentInstance_ResolveCandidatesFromRawCodexModel(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
				ModelName: "gpt-5.4-mini",
			},
		},
		Runtime: config.RuntimeConfig{
			Codex: config.CodexRuntimeConfig{
				DefaultModel:    "gpt-5.4",
				DefaultThinking: "medium",
			},
		},
	}

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})
	if len(agent.Candidates) != 1 {
		t.Fatalf("len(Candidates) = %d, want 1", len(agent.Candidates))
	}
	if agent.Candidates[0].Provider != "codex" {
		t.Fatalf("candidate provider = %q, want %q", agent.Candidates[0].Provider, "codex")
	}
	if agent.Candidates[0].Model != "gpt-5.4-mini" {
		t.Fatalf("candidate model = %q, want %q", agent.Candidates[0].Model, "gpt-5.4-mini")
	}
}

func TestNewAgentInstance_AllowsMediaTempDirForReadListAndExec(t *testing.T) {
	workspace := t.TempDir()
	mediaDir := media.TempDir()
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(mediaDir) error = %v", err)
	}

	mediaFile, err := os.CreateTemp(mediaDir, "instance-tool-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp(mediaDir) error = %v", err)
	}
	mediaPath := mediaFile.Name()
	if _, err := mediaFile.WriteString("attachment content"); err != nil {
		mediaFile.Close()
		t.Fatalf("WriteString(mediaFile) error = %v", err)
	}
	if err := mediaFile.Close(); err != nil {
		t.Fatalf("Close(mediaFile) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(mediaPath) })

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:           workspace,
				ModelName:           "test-model",
				RestrictToWorkspace: true,
			},
		},
		Tools: config.ToolsConfig{
			ReadFile: config.ReadFileToolConfig{Enabled: true},
			ListDir:  config.ToolConfig{Enabled: true},
			Exec: config.ExecConfig{
				ToolConfig:         config.ToolConfig{Enabled: true},
				EnableDenyPatterns: true,
				AllowRemote:        true,
			},
		},
	}

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})

	readTool, ok := agent.Tools.Get("read_file")
	if !ok {
		t.Fatal("read_file tool not registered")
	}
	readResult := readTool.Execute(context.Background(), map[string]any{"path": mediaPath})
	if readResult.IsError {
		t.Fatalf("read_file should allow media temp dir, got: %s", readResult.ForLLM)
	}
	if !strings.Contains(readResult.ForLLM, "attachment content") {
		t.Fatalf("read_file output missing media content: %s", readResult.ForLLM)
	}

	listTool, ok := agent.Tools.Get("list_dir")
	if !ok {
		t.Fatal("list_dir tool not registered")
	}
	listResult := listTool.Execute(context.Background(), map[string]any{"path": mediaDir})
	if listResult.IsError {
		t.Fatalf("list_dir should allow media temp dir, got: %s", listResult.ForLLM)
	}
	if !strings.Contains(listResult.ForLLM, filepath.Base(mediaPath)) {
		t.Fatalf("list_dir output missing media file: %s", listResult.ForLLM)
	}

	execTool, ok := agent.Tools.Get("exec")
	if !ok {
		t.Fatal("exec tool not registered")
	}
	execResult := execTool.Execute(context.Background(), map[string]any{
		"action":  "run",
		"command": "cat " + filepath.Base(mediaPath),
		"cwd":     mediaDir,
	})
	if execResult.IsError {
		t.Fatalf("exec should allow media temp dir, got: %s", execResult.ForLLM)
	}
	if !strings.Contains(execResult.ForLLM, "attachment content") {
		t.Fatalf("exec output missing media content: %s", execResult.ForLLM)
	}
}

func TestBuildAllowReadPatterns_AllowsExternalCodexSkillRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workspace := t.TempDir()
	skillFile := filepath.Join(home, ".agents", "skills", "using-superpowers", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill dir) error = %v", err)
	}
	if err := os.WriteFile(skillFile, []byte("skill body"), 0o644); err != nil {
		t.Fatalf("WriteFile(skill file) error = %v", err)
	}

	patterns := buildAllowReadPatterns(&config.Config{})
	tool := tools.NewReadFileTool(workspace, true, tools.MaxReadFileSize, patterns)

	result := tool.Execute(context.Background(), map[string]any{"path": skillFile})
	if result.IsError {
		t.Fatalf("read_file should allow external codex skill roots, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "skill body") {
		t.Fatalf("read_file output missing skill body: %s", result.ForLLM)
	}
}

// TestPopulateCandidateProviders_NilCfgIsNoop verifies that passing a nil
// config does not panic and leaves the output map empty.
func TestPopulateCandidateProviders_NilCfgIsNoop(t *testing.T) {
	out := map[string]providers.LLMProvider{}
	populateCandidateProvidersFromNames(nil, t.TempDir(), []string{"gpt-4o"}, out)
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(out))
	}
}

// TestPopulateCandidateProviders_SkipsExistingKeys verifies that a key already
// present in the output map is not overwritten.
func TestPopulateCandidateProviders_SkipsExistingKeys(t *testing.T) {
	existing := &mockProvider{}
	key := providers.ModelKey("codex", "gpt-5.4")
	out := map[string]providers.LLMProvider{key: existing}

	cfg := config.DefaultConfig()
	populateCandidateProvidersFromNames(cfg, t.TempDir(), []string{"gpt-5.4"}, out)

	if out[key] != existing {
		t.Fatal("existing provider entry was overwritten; expected it to be preserved")
	}
}

func TestPopulateCandidateProviders_ResolvesRawCodexModel(t *testing.T) {
	workspace := t.TempDir()
	out := map[string]providers.LLMProvider{}

	cfg := config.DefaultConfig()
	populateCandidateProvidersFromNames(cfg, workspace, []string{"gpt-5.4-mini"}, out)

	key := providers.ModelKey("codex", "gpt-5.4-mini")
	if out[key] == nil {
		t.Fatalf("expected CandidateProviders[%q] to be populated for codex model", key)
	}
}

func TestPopulateCandidateProviders_ResolvesRuntimeDeepSeekFallback(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")
	out := map[string]providers.LLMProvider{}

	cfg := config.DefaultConfig()
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-chat"
	cfg.Runtime.Fallback.DeepSeek.APIBase = "https://api.deepseek.com/v1"
	populateCandidateProvidersFromNames(cfg, workspace, []string{"deepseek-chat"}, out)

	key := providers.ModelKey("deepseek", "deepseek-chat")
	if out[key] == nil {
		t.Fatalf("expected CandidateProviders[%q] to be populated for deepseek fallback", key)
	}
}

func TestNewAgentInstance_ConfiguresRuntimeDeepSeekFallbackProvider(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.ModelName = "gpt-5.4"
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-chat"
	cfg.Runtime.Fallback.DeepSeek.APIBase = "https://api.deepseek.com/v1"

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})

	if agent.DeepSeekFallback == nil {
		t.Fatal("DeepSeekFallback = nil, want configured provider")
	}
	if agent.DeepSeekFallbackModel != "deepseek-chat" {
		t.Fatalf("DeepSeekFallbackModel = %q, want %q", agent.DeepSeekFallbackModel, "deepseek-chat")
	}
}

func TestNewAgentInstance_SkipsRuntimeDeepSeekFallbackWithoutAPIKey(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.ModelName = "gpt-5.4"
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-chat"
	cfg.Runtime.Fallback.DeepSeek.APIBase = "https://api.deepseek.com/v1"

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})

	if agent.DeepSeekFallback != nil {
		t.Fatalf("DeepSeekFallback = %#v, want nil", agent.DeepSeekFallback)
	}
	if agent.DeepSeekFallbackModel != "" {
		t.Fatalf("DeepSeekFallbackModel = %q, want empty", agent.DeepSeekFallbackModel)
	}
}

// TestPopulateCandidateProviders_EmptyNamesIsNoop verifies the early-exit
// path when the names slice is empty.
func TestPopulateCandidateProviders_EmptyNamesIsNoop(t *testing.T) {
	out := map[string]providers.LLMProvider{}
	cfg := config.DefaultConfig()
	populateCandidateProvidersFromNames(cfg, t.TempDir(), nil, out)
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(out))
	}
}

func TestNewAgentInstance_IgnoresDeprecatedConfiguredFallbacks(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.ModelName = "gpt-5.4"
	cfg.Agents.Defaults.ModelFallbacks = []string{"gpt-5.4-mini", "deepseek-chat"}
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-chat"
	cfg.Runtime.Fallback.DeepSeek.APIBase = "https://api.deepseek.com/v1"

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})

	if len(agent.Fallbacks) != 0 {
		t.Fatalf("Fallbacks = %v, want deprecated fallback arrays to be ignored", agent.Fallbacks)
	}
	if len(agent.Candidates) != 1 {
		t.Fatalf("len(Candidates) = %d, want 1 primary candidate", len(agent.Candidates))
	}

	deprecatedKeys := []string{
		providers.ModelKey("codex", "gpt-5.4-mini"),
		providers.ModelKey("deepseek", "deepseek-chat"),
	}

	for _, key := range deprecatedKeys {
		if _, ok := agent.CandidateProviders[key]; ok {
			t.Fatalf("CandidateProviders[%q] should not be populated from deprecated fallback config", key)
		}
	}

	if agent.DeepSeekFallback == nil {
		t.Fatal("DeepSeekFallback = nil, want runtime fallback provider to remain configured")
	}
}

func TestNewAgentInstance_ReadFileModeSelectsSchema(t *testing.T) {
	workspace := t.TempDir()

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: workspace,
				ModelName: "test-model",
			},
		},
		Tools: config.ToolsConfig{
			ReadFile: config.ReadFileToolConfig{
				Enabled:         true,
				Mode:            config.ReadFileModeLines,
				MaxReadFileSize: 4096,
			},
		},
	}

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})
	readTool, ok := agent.Tools.Get("read_file")
	if !ok {
		t.Fatal("read_file tool not registered")
	}

	params := readTool.Parameters()
	props, _ := params["properties"].(map[string]any)
	if _, ok := props["start_line"]; !ok {
		t.Fatalf("expected line-mode schema to expose start_line, got %#v", props)
	}
	if _, ok := props["max_lines"]; !ok {
		t.Fatalf("expected line-mode schema to expose max_lines, got %#v", props)
	}
	if _, ok := props["offset"]; ok {
		t.Fatalf("did not expect line-mode schema to expose offset, got %#v", props)
	}
	if _, ok := props["length"]; ok {
		t.Fatalf("did not expect line-mode schema to expose length, got %#v", props)
	}
}

func TestNewAgentInstance_InvalidExecConfigDoesNotExit(t *testing.T) {
	workspace := t.TempDir()

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: workspace,
				ModelName: "test-model",
			},
		},
		Tools: config.ToolsConfig{
			ReadFile: config.ReadFileToolConfig{Enabled: true},
			Exec: config.ExecConfig{
				ToolConfig:         config.ToolConfig{Enabled: true},
				EnableDenyPatterns: true,
				CustomDenyPatterns: []string{"[invalid-regex"},
			},
		},
	}

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})
	if agent == nil {
		t.Fatal("expected agent instance, got nil")
	}

	if _, ok := agent.Tools.Get("exec"); ok {
		t.Fatal("exec tool should not be registered when exec config is invalid")
	}

	if _, ok := agent.Tools.Get("read_file"); !ok {
		t.Fatal("read_file tool should still be registered")
	}
}
