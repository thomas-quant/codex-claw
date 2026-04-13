package commands

import (
	"time"

	"github.com/thomas-quant/codex-claw/pkg/config"
)

// ModelInfo describes a runtime-visible model entry.
type ModelInfo struct {
	Name     string
	Provider string
}

// StatusSnapshot is the runtime-visible per-thread state used by command
// handlers. It stays small and presentation-oriented so the command package
// does not need to know about codexruntime internals.
type StatusSnapshot struct {
	ThreadID          string
	Model             string
	Provider          string
	ThinkingMode      string
	FastEnabled       bool
	LastUserMessageAt time.Time
	LastCompactionAt  time.Time
	ForceFreshThread  bool
	RecoveryState     string
}

// Runtime provides runtime dependencies to command handlers. It is constructed
// per-request by the agent loop so that per-request state (like session scope)
// can coexist with long-lived callbacks (like GetModelInfo).
type Runtime struct {
	Config             *config.Config
	GetModelInfo       func() (name, provider string)
	ListModels         func() []ModelInfo
	ReadStatus         func() (StatusSnapshot, bool)
	ListAgentIDs       func() []string
	ListDefinitions    func() []Definition
	ListSkillNames     func() []string
	GetEnabledChannels func() []string
	GetActiveTurn      func() any // Returning any to avoid circular dependency with agent package
	SwitchModel        func(value string) (oldModel string, err error)
	SetModel           func(value string) (oldModel string, err error)
	SetThinking        func(value string) (oldThinking string, err error)
	ToggleFast         func() (enabled bool, err error)
	CompactThread      func() error
	ResetThread        func() error
	SwitchChannel      func(value string) error
	ClearHistory       func() error
	ReloadConfig       func() error
}
