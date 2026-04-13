package agent

import (
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

const defaultRuntimeProvider = "codex"

func runtimeDefaultProvider(defaultProvider string) string {
	defaultProvider = strings.TrimSpace(defaultProvider)
	if defaultProvider == "" {
		return defaultRuntimeProvider
	}
	return providers.NormalizeProvider(defaultProvider)
}

func ensureProtocolModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.Contains(model, "/") {
		return model
	}
	return defaultRuntimeProvider + "/" + model
}

func modelConfigIdentityKey(mc *config.ModelConfig) string {
	if mc == nil {
		return ""
	}
	if name := strings.TrimSpace(mc.ModelName); name != "" {
		return "model_name:" + name
	}
	return ""
}

func candidateFromModelConfig(
	defaultProvider string,
	mc *config.ModelConfig,
) (providers.FallbackCandidate, bool) {
	if mc == nil {
		return providers.FallbackCandidate{}, false
	}

	ref := providers.ParseModelRef(ensureProtocolModel(mc.Model), defaultProvider)
	if ref == nil {
		return providers.FallbackCandidate{}, false
	}

	return providers.FallbackCandidate{
		Provider:    ref.Provider,
		Model:       ref.Model,
		RPM:         mc.RPM,
		IdentityKey: modelConfigIdentityKey(mc),
	}, true
}

func lookupModelConfigByRef(cfg *config.Config, raw string) *config.ModelConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" || cfg == nil {
		return nil
	}

	if mc, ok := providers.DeepSeekFallbackModelConfig(cfg); ok {
		fullModel := strings.TrimSpace(mc.Model)
		if fullModel == raw || strings.TrimSpace(mc.ModelName) == raw {
			return mc
		}
		_, modelID := providers.ExtractProtocol(fullModel)
		if modelID == raw {
			return mc
		}
	}

	return nil
}

func resolveModelCandidate(
	cfg *config.Config,
	defaultProvider string,
	raw string,
) (providers.FallbackCandidate, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return providers.FallbackCandidate{}, false
	}

	if mc := lookupModelConfigByRef(cfg, raw); mc != nil {
		return candidateFromModelConfig(runtimeDefaultProvider(defaultProvider), mc)
	}

	ref := providers.ParseModelRef(ensureProtocolModel(raw), runtimeDefaultProvider(defaultProvider))
	if ref == nil {
		return providers.FallbackCandidate{}, false
	}

	return providers.FallbackCandidate{
		Provider: ref.Provider,
		Model:    ref.Model,
	}, true
}

func resolveModelCandidates(
	cfg *config.Config,
	defaultProvider string,
	primary string,
	fallbacks []string,
) []providers.FallbackCandidate {
	seen := make(map[string]bool)
	candidates := make([]providers.FallbackCandidate, 0, 1+len(fallbacks))

	addCandidate := func(raw string) {
		candidate, ok := resolveModelCandidate(cfg, defaultProvider, raw)
		if !ok {
			return
		}

		key := candidate.StableKey()
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}

	addCandidate(primary)
	for _, fallback := range fallbacks {
		addCandidate(fallback)
	}

	return candidates
}

func resolvedCandidateModel(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 && strings.TrimSpace(candidates[0].Model) != "" {
		return candidates[0].Model
	}
	return fallback
}

func resolvedCandidateProvider(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 && strings.TrimSpace(candidates[0].Provider) != "" {
		return candidates[0].Provider
	}
	return fallback
}

func resolvedModelConfig(cfg *config.Config, modelName, workspace string) (*config.ModelConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	raw := strings.TrimSpace(modelName)
	if raw == "" {
		return nil, fmt.Errorf("model name is required")
	}

	if modelCfg := lookupModelConfigByRef(cfg, raw); modelCfg != nil {
		clone := *modelCfg
		if clone.Workspace == "" {
			clone.Workspace = workspace
		}
		return &clone, nil
	}

	fullModel := ensureProtocolModel(raw)
	protocol, _ := providers.ExtractProtocol(fullModel)
	modelCfg := &config.ModelConfig{
		ModelName: raw,
		Model:     fullModel,
		Workspace: workspace,
	}
	if protocol == "codex" {
		modelCfg.ThinkingLevel = cfg.Runtime.Codex.DefaultThinking
	}
	return modelCfg, nil
}
