// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// CreateProvider creates the primary runtime provider.
// The codex-first runtime uses the Codex app-server only and takes its model
// from the runtime config instead of legacy model/provider aliases.
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}

	model := strings.TrimSpace(cfg.Runtime.Codex.DefaultModel)
	if model == "" {
		return nil, "", fmt.Errorf("runtime.codex.default_model is required")
	}

	workspace := cfg.WorkspacePath()
	if workspace == "" {
		workspace = "."
	}

	return NewCodexAppServerProvider(newCodexAppServerRunner(workspace, 0)), model, nil
}
