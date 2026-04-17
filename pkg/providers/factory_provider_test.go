package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
	"github.com/thomas-quant/codex-claw/pkg/codexruntime"
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

func TestCreateProviderFromConfig_UsesManagedCodexRunner(t *testing.T) {
	t.Setenv(config.EnvHome, t.TempDir())

	originalClientFactory := newCodexRunnerClient
	originalDefaultCodexHome := defaultCodexCLIHome
	defer func() {
		newCodexRunnerClient = originalClientFactory
		defaultCodexCLIHome = originalDefaultCodexHome
	}()

	layout := codexaccounts.ResolveLayout(config.GetHome())
	if err := os.MkdirAll(filepath.Dir(layout.LiveAuthFile), 0o700); err != nil {
		t.Fatalf("MkdirAll(live auth dir) error = %v", err)
	}
	if err := os.WriteFile(layout.LiveAuthFile, []byte(`{"token":"managed"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(live auth) error = %v", err)
	}

	var gotWorkspace string
	var gotTimeout int
	var gotEnv map[string]string
	newCodexRunnerClient = func(workspace string, requestTimeoutSeconds int, envOverrides map[string]string) codexruntime.RunnerClient {
		gotWorkspace = workspace
		gotTimeout = requestTimeoutSeconds
		gotEnv = map[string]string{}
		for key, value := range envOverrides {
			gotEnv[key] = value
		}
		return &stubRunnerClient{}
	}

	cfg := &config.ModelConfig{
		ModelName:      "codex-managed",
		Model:          "codex/gpt-5.4",
		Workspace:      "/tmp/workspace-a",
		RequestTimeout: 42,
	}

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	codexProvider, ok := provider.(*CodexAppServerProvider)
	if !ok {
		t.Fatalf("CreateProviderFromConfig() provider type = %T, want *CodexAppServerProvider", provider)
	}
	if _, ok := codexProvider.runner.(*codexaccounts.Coordinator); !ok {
		t.Fatalf("runner type = %T, want *codexaccounts.Coordinator", codexProvider.runner)
	}
	if modelID != "gpt-5.4" {
		t.Fatalf("CreateProviderFromConfig() modelID = %q, want %q", modelID, "gpt-5.4")
	}
	if gotWorkspace != cfg.Workspace {
		t.Fatalf("workspace = %q, want %q", gotWorkspace, cfg.Workspace)
	}
	if gotTimeout != cfg.RequestTimeout {
		t.Fatalf("request timeout = %d, want %d", gotTimeout, cfg.RequestTimeout)
	}

	if gotEnv["CODEX_HOME"] != layout.CodexHome {
		t.Fatalf("CODEX_HOME = %q, want %q", gotEnv["CODEX_HOME"], layout.CodexHome)
	}
}

func TestCreateProviderFromConfig_FallsBackToDefaultCodexHomeWhenManagedHomeMissing(t *testing.T) {
	t.Setenv(config.EnvHome, t.TempDir())

	originalClientFactory := newCodexRunnerClient
	originalDefaultCodexHome := defaultCodexCLIHome
	defer func() {
		newCodexRunnerClient = originalClientFactory
		defaultCodexCLIHome = originalDefaultCodexHome
	}()

	fallbackHome := t.TempDir()
	defaultCodexCLIHome = func() string { return fallbackHome }

	var gotEnv map[string]string
	newCodexRunnerClient = func(workspace string, requestTimeoutSeconds int, envOverrides map[string]string) codexruntime.RunnerClient {
		gotEnv = map[string]string{}
		for key, value := range envOverrides {
			gotEnv[key] = value
		}
		return &stubRunnerClient{}
	}

	cfg := &config.ModelConfig{
		ModelName: "codex-managed",
		Model:     "codex/gpt-5.4",
	}

	if _, _, err := CreateProviderFromConfig(cfg); err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if gotEnv["CODEX_HOME"] != fallbackHome {
		t.Fatalf("CODEX_HOME = %q, want %q", gotEnv["CODEX_HOME"], fallbackHome)
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

func TestDeepSeekFallbackModelConfig_DisabledWithoutAPIKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")

	cfg := config.DefaultConfig()
	cfg.Runtime.Fallback.DeepSeek.Enabled = true
	cfg.Runtime.Fallback.DeepSeek.Model = "deepseek-chat"

	modelCfg, ok := DeepSeekFallbackModelConfig(cfg)
	if ok {
		t.Fatalf("DeepSeekFallbackModelConfig() ok = true, want false (cfg=%#v)", modelCfg)
	}
	if modelCfg != nil {
		t.Fatalf("DeepSeekFallbackModelConfig() = %#v, want nil", modelCfg)
	}
}

type stubRunnerClient struct{}

func (stubRunnerClient) Start(context.Context) error {
	return nil
}

func (stubRunnerClient) ResumeThread(context.Context, string, []codexruntime.DynamicToolDefinition) error {
	return nil
}

func (stubRunnerClient) StartThread(context.Context, string, []codexruntime.DynamicToolDefinition) (string, error) {
	return "", nil
}

func (stubRunnerClient) RunTextTurn(context.Context, codexruntime.RunTurnRequest) (string, error) {
	return "", nil
}

func (stubRunnerClient) Restart(context.Context) error {
	return nil
}

func (stubRunnerClient) StartNativeCompaction(context.Context, string) error {
	return nil
}

func (stubRunnerClient) ListModels(context.Context) ([]codexruntime.ModelCatalogEntry, error) {
	return nil, nil
}

func (stubRunnerClient) ReadAccount(context.Context, bool) (codexruntime.AccountSnapshot, error) {
	return codexruntime.AccountSnapshot{}, nil
}

func (stubRunnerClient) ReadRateLimits(context.Context) ([]codexruntime.RateLimitSnapshot, error) {
	return nil, nil
}

func (stubRunnerClient) Close() error {
	return nil
}

func (stubRunnerClient) Status() codexruntime.ClientStatus {
	return codexruntime.ClientStatus{}
}
