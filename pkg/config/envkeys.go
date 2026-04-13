// Codex-Claw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Codex-Claw contributors

package config

import (
	"os"
	"path/filepath"

	"github.com/sipeed/codex-claw/pkg"
)

// Runtime environment variable keys for the codex-claw process.
// These control the location of files and binaries at runtime and are read
// directly via os.Getenv / os.LookupEnv. All codex-claw-specific keys use the
// CODEX_CLAW_ prefix. Reference these constants instead of inline string
// literals to keep all supported knobs visible in one place and to prevent
// typos.
const (
	// EnvHome overrides the base directory for all codex-claw data
	// (config, workspace, skills, runtime state, …).
	// Default: ~/.codex-claw
	EnvHome = "CODEX_CLAW_HOME"

	// EnvConfig overrides the full path to the JSON config file.
	// Default: $CODEX_CLAW_HOME/config.json
	EnvConfig = "CODEX_CLAW_CONFIG"

	// EnvBuiltinSkills overrides the directory from which built-in
	// skills are loaded.
	// Default: <cwd>/skills
	EnvBuiltinSkills = "CODEX_CLAW_BUILTIN_SKILLS"

	// EnvBinary overrides the path to the codex-claw executable.
	// Used by local helper flows that need to re-exec the current binary.
	// Default: resolved from the same directory as the current executable.
	EnvBinary = "CODEX_CLAW_BINARY"

	// EnvGatewayHost overrides the host address for the gateway server.
	// Default: "127.0.0.1"
	EnvGatewayHost = "CODEX_CLAW_GATEWAY_HOST"
)

func GetHome() string {
	homePath, _ := os.UserHomeDir()
	if codexClawHome := os.Getenv(EnvHome); codexClawHome != "" {
		homePath = codexClawHome
	} else if homePath != "" {
		homePath = filepath.Join(homePath, pkg.DefaultCodexClawHome)
	}
	if homePath == "" {
		homePath = "."
	}
	return homePath
}
