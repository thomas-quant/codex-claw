package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestCreateProvider_UsesExplicitStartupModelOverride(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runtime.Codex.DefaultModel = "gpt-5.4-codex"
	cfg.Agents.Defaults.ModelName = "openai/gpt-4o"
	cfg.Agents.List = []config.AgentConfig{
		{
			ID:      "main",
			Default: true,
			Model: &config.AgentModelConfig{
				Primary: "openai/gpt-3.5",
			},
		},
	}

	provider, model, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}

	if provider == nil {
		t.Fatal("CreateProvider() returned nil provider")
	}
	if _, ok := provider.(*CodexAppServerProvider); !ok {
		t.Fatalf("CreateProvider() provider type = %T, want *CodexAppServerProvider", provider)
	}
	if model != "openai/gpt-4o" {
		t.Fatalf("CreateProvider() model = %q, want %q", model, "openai/gpt-4o")
	}

}

func TestCreateProvider_UsesCodexRuntimeDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runtime.Codex.DefaultModel = "gpt-5.4-codex"

	cfg.Agents.List = nil
	provider, model, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	if _, ok := provider.(*CodexAppServerProvider); !ok {
		t.Fatalf("CreateProvider() provider type = %T, want *CodexAppServerProvider", provider)
	}
	if model != "gpt-5.4-codex" {
		t.Fatalf("CreateProvider() model = %q, want %q", model, "gpt-5.4-codex")
	}
}

func TestCreateDeepSeekFallbackCandidate_UsesRuntimeBlock(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-reasoner"
	cfg.Runtime.Fallback.DeepSeek.APIBase = "https://api.deepseek.example/v1"

	modelCfg, ok := DeepSeekFallbackModelConfig(cfg)
	if !ok {
		t.Fatal("DeepSeekFallbackModelConfig() ok = false, want true")
	}
	if modelCfg.ModelName != "deepseek-fallback" {
		t.Fatalf("ModelName = %q, want %q", modelCfg.ModelName, "deepseek-fallback")
	}
	if modelCfg.Model != "deepseek/deepseek-reasoner" {
		t.Fatalf("Model = %q, want %q", modelCfg.Model, "deepseek/deepseek-reasoner")
	}
	if modelCfg.APIBase != "https://api.deepseek.example/v1" {
		t.Fatalf("APIBase = %q, want %q", modelCfg.APIBase, "https://api.deepseek.example/v1")
	}

	provider, modelID, err := CreateProviderFromConfig(modelCfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if _, ok := provider.(*HTTPProvider); !ok {
		t.Fatalf("CreateProviderFromConfig() provider type = %T, want *HTTPProvider", provider)
	}
	if modelID != "deepseek-reasoner" {
		t.Fatalf("CreateProviderFromConfig() modelID = %q, want %q", modelID, "deepseek-reasoner")
	}
}
