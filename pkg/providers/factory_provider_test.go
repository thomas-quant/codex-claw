package providers

import (
	"os"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/config"
)

func TestExtractProtocol(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		wantProtocol string
		wantModelID  string
	}{
		{
			name:         "openai with prefix",
			model:        "openai/gpt-4o",
			wantProtocol: "openai",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "no prefix defaults to openai",
			model:        "gpt-4o",
			wantProtocol: "openai",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "deepseek with prefix",
			model:        "deepseek/deepseek-chat",
			wantProtocol: "deepseek",
			wantModelID:  "deepseek-chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, modelID := ExtractProtocol(tt.model)
			if protocol != tt.wantProtocol {
				t.Fatalf("ExtractProtocol(%q) protocol = %q, want %q", tt.model, protocol, tt.wantProtocol)
			}
			if modelID != tt.wantModelID {
				t.Fatalf("ExtractProtocol(%q) modelID = %q, want %q", tt.model, modelID, tt.wantModelID)
			}
		})
	}
}

func TestCreateProviderFromConfig_UsesHTTPProviderForOpenAI(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-openai",
		Model:     "openai/gpt-4o",
		APIBase:   "https://api.example.com/v1",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if _, ok := provider.(*HTTPProvider); !ok {
		t.Fatalf("CreateProviderFromConfig() provider type = %T, want *HTTPProvider", provider)
	}
	if modelID != "gpt-4o" {
		t.Fatalf("CreateProviderFromConfig() modelID = %q, want %q", modelID, "gpt-4o")
	}
}

func TestDeepSeekFallbackModelConfig_UsesRuntimeDefaults(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")
	_ = os.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")
	cfg := config.DefaultConfig()
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = ""
	cfg.Runtime.Fallback.DeepSeek.APIBase = ""

	modelCfg, ok := DeepSeekFallbackModelConfig(cfg)
	if !ok {
		t.Fatal("DeepSeekFallbackModelConfig() ok = false, want true")
	}
	if modelCfg.Model != "deepseek/deepseek-chat" {
		t.Fatalf("Model = %q, want %q", modelCfg.Model, "deepseek/deepseek-chat")
	}
	if modelCfg.APIBase != "https://api.deepseek.com/v1" {
		t.Fatalf("APIBase = %q, want %q", modelCfg.APIBase, "https://api.deepseek.com/v1")
	}
	if modelCfg.APIKey() != "deepseek-test-key" {
		t.Fatalf("APIKey() = %q, want %q", modelCfg.APIKey(), "deepseek-test-key")
	}
}
