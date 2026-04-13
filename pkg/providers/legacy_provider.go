// Codex Claw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Codex Claw contributors

package providers

import (
	"fmt"
	"strings"

	"github.com/sipeed/codex-claw/pkg/config"
)

// CreateProvider creates the primary runtime provider.
// The codex-first runtime uses the Codex app-server only and takes its model
// from the runtime config instead of legacy model/provider aliases.
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}

	model := strings.TrimSpace(cfg.Agents.Defaults.GetModelName())
	if model == "" {
		model = explicitAgentModel(cfg)
	}
	if model == "" {
		model = strings.TrimSpace(cfg.Runtime.Codex.DefaultModel)
	}
	if model == "" {
		return nil, "", fmt.Errorf("runtime.codex.default_model is required")
	}

	workspace := cfg.WorkspacePath()
	if workspace == "" {
		workspace = "."
	}

	return NewCodexAppServerProvider(newCodexAppServerRunner(workspace, 0)), model, nil
}

func explicitAgentModel(cfg *config.Config) string {
	if cfg == nil || len(cfg.Agents.List) == 0 {
		return ""
	}

	for i := range cfg.Agents.List {
		agent := &cfg.Agents.List[i]
		if !agent.Default {
			continue
		}
		if agent.Model != nil {
			if model := strings.TrimSpace(agent.Model.Primary); model != "" {
				return model
			}
		}
	}

	if len(cfg.Agents.List) == 1 {
		agent := &cfg.Agents.List[0]
		if agent.Model != nil {
			return strings.TrimSpace(agent.Model.Primary)
		}
	}

	return ""
}
