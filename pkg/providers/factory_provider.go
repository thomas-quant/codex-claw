// Codex Claw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Codex Claw contributors

package providers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/codex-claw/pkg/codexruntime"
	"github.com/sipeed/codex-claw/pkg/config"
)

type protocolMeta struct {
	defaultAPIBase     string
	emptyAPIKeyAllowed bool
}

var protocolMetaByName = map[string]protocolMeta{
	"openai":                   {defaultAPIBase: "https://api.openai.com/v1"},
	"deepseek":                 {defaultAPIBase: "https://api.deepseek.com/v1"},
	"litellm":                  {defaultAPIBase: "http://localhost:4000/v1"},
	"lmstudio":                 {defaultAPIBase: "http://localhost:1234/v1", emptyAPIKeyAllowed: true},
	"openrouter":               {defaultAPIBase: "https://openrouter.ai/api/v1"},
	"groq":                     {defaultAPIBase: "https://api.groq.com/openai/v1"},
	"zhipu":                    {defaultAPIBase: "https://open.bigmodel.cn/api/paas/v4"},
	"nvidia":                   {defaultAPIBase: "https://integrate.api.nvidia.com/v1"},
	"venice":                   {defaultAPIBase: "https://api.venice.ai/api/v1"},
	"ollama":                   {defaultAPIBase: "http://localhost:11434/v1", emptyAPIKeyAllowed: true},
	"moonshot":                 {defaultAPIBase: "https://api.moonshot.cn/v1"},
	"shengsuanyun":             {defaultAPIBase: "https://router.shengsuanyun.com/api/v1"},
	"cerebras":                 {defaultAPIBase: "https://api.cerebras.ai/v1"},
	"vivgrid":                  {defaultAPIBase: "https://api.vivgrid.com/v1"},
	"volcengine":               {defaultAPIBase: "https://ark.cn-beijing.volces.com/api/v3"},
	"qwen":                     {defaultAPIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
	"qwen-intl":                {defaultAPIBase: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
	"qwen-international":       {defaultAPIBase: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
	"dashscope-intl":           {defaultAPIBase: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
	"qwen-us":                  {defaultAPIBase: "https://dashscope-us.aliyuncs.com/compatible-mode/v1"},
	"dashscope-us":             {defaultAPIBase: "https://dashscope-us.aliyuncs.com/compatible-mode/v1"},
	"coding-plan":              {defaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/v1"},
	"alibaba-coding":           {defaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/v1"},
	"qwen-coding":              {defaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/v1"},
	"coding-plan-anthropic":    {defaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic"},
	"alibaba-coding-anthropic": {defaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic"},
	"vllm":                     {defaultAPIBase: "http://localhost:8000/v1", emptyAPIKeyAllowed: true},
	"mistral":                  {defaultAPIBase: "https://api.mistral.ai/v1"},
	"avian":                    {defaultAPIBase: "https://api.avian.io/v1"},
	"minimax":                  {defaultAPIBase: "https://api.minimaxi.com/v1"},
	"longcat":                  {defaultAPIBase: "https://api.longcat.chat/openai"},
	"modelscope":               {defaultAPIBase: "https://api-inference.modelscope.cn/v1"},
	"mimo":                     {defaultAPIBase: "https://api.xiaomimimo.com/v1"},
	"novita":                   {defaultAPIBase: "https://api.novita.ai/openai"},
}

var newCodexAppServerRunner = func(workspace string, requestTimeoutSeconds int) codexAppServerRunner {
	bindings := codexruntime.NewBindingStore(filepath.Join(workspace, "codex"))
	client := codexruntime.NewStdIOClient(
		"codex",
		[]string{"app-server", "--listen", "stdio://"},
		workspace,
		time.Duration(requestTimeoutSeconds)*time.Second,
	)
	return codexruntime.NewRunner(client, bindings)
}

// ExtractProtocol extracts the protocol prefix and model identifier from a model string.
// If no prefix is specified, it defaults to "openai".
func ExtractProtocol(model string) (protocol, modelID string) {
	model = strings.TrimSpace(model)
	protocol, modelID, found := strings.Cut(model, "/")
	if !found {
		return "openai", model
	}
	return protocol, modelID
}

// ResolveAPIBase returns the configured API base, or the protocol default when
// the model uses an HTTP-based provider family with a known default endpoint.
func ResolveAPIBase(cfg *config.ModelConfig) string {
	if cfg == nil {
		return ""
	}
	if apiBase := strings.TrimSpace(cfg.APIBase); apiBase != "" {
		return strings.TrimRight(apiBase, "/")
	}
	protocol, _ := ExtractProtocol(cfg.Model)
	return strings.TrimRight(getDefaultAPIBase(protocol), "/")
}

// CreateProviderFromConfig creates a provider based on the ModelConfig.
// The codex-first runtime keeps only the Codex app-server and HTTP/OpenAI-compatible paths.
func CreateProviderFromConfig(cfg *config.ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}
	if cfg.Model == "" {
		return nil, "", fmt.Errorf("model is required")
	}

	protocol, modelID := ExtractProtocol(cfg.Model)
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = fmt.Sprintf("codex-claw/%s", config.Version)
	}

	switch protocol {
	case "codex":
		workspace := cfg.Workspace
		if workspace == "" {
			workspace = "."
		}
		return NewCodexAppServerProvider(newCodexAppServerRunner(workspace, cfg.RequestTimeout)), modelID, nil

	case "openai", "deepseek", "litellm", "lmstudio", "openrouter", "groq", "zhipu", "nvidia", "venice",
		"ollama", "moonshot", "shengsuanyun", "cerebras", "vivgrid", "volcengine", "vllm", "qwen",
		"qwen-intl", "qwen-international", "dashscope-intl", "qwen-us", "dashscope-us", "mistral",
		"avian", "longcat", "modelscope", "novita", "coding-plan", "alibaba-coding", "qwen-coding", "mimo":
		if cfg.APIKey() == "" && cfg.APIBase == "" && !isEmptyAPIKeyAllowed(protocol) {
			return nil, "", fmt.Errorf("api_key or api_base is required for HTTP-based protocol %q", protocol)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey(),
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			userAgent,
			cfg.RequestTimeout,
			cfg.ExtraBody,
			cfg.CustomHeaders,
		), modelID, nil

	default:
		return nil, "", fmt.Errorf("unknown protocol %q in model %q", protocol, cfg.Model)
	}
}

func isEmptyAPIKeyAllowed(protocol string) bool {
	meta, ok := protocolMetaByName[protocol]
	return ok && meta.emptyAPIKeyAllowed
}

// IsEmptyAPIKeyAllowedForProtocol reports whether a protocol allows requests
// without api_key when using its default local endpoint.
func IsEmptyAPIKeyAllowedForProtocol(protocol string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	return isEmptyAPIKeyAllowed(protocol)
}

// DefaultAPIBaseForProtocol returns the configured default API base for a protocol.
// It returns empty string if the protocol has no default base.
func DefaultAPIBaseForProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	return getDefaultAPIBase(protocol)
}

// getDefaultAPIBase returns the default API base URL for a given protocol.
func getDefaultAPIBase(protocol string) string {
	meta, ok := protocolMetaByName[protocol]
	if !ok {
		return ""
	}
	return meta.defaultAPIBase
}
