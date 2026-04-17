package channels

import (
	"context"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/bus"
	"github.com/thomas-quant/codex-claw/pkg/config"
)

func TestNewManager_OnlyInitializesTelegramAndDiscord(t *testing.T) {
	factoriesMu.Lock()
	originalFactories := make(map[string]ChannelFactory, len(factories))
	for name, factory := range factories {
		originalFactories[name] = factory
	}
	factories = map[string]ChannelFactory{}
	factoriesMu.Unlock()
	t.Cleanup(func() {
		factoriesMu.Lock()
		factories = originalFactories
		factoriesMu.Unlock()
	})

	registerStubFactory := func(name string) {
		RegisterFactory(name, func(cfg *config.Config, b *bus.MessageBus) (Channel, error) {
			return &mockChannel{
				BaseChannel: *NewBaseChannel(name, nil, b, nil),
			}, nil
		})
	}

	registerStubFactory("telegram")
	registerStubFactory("discord")
	registerStubFactory("unsupported-alpha")
	registerStubFactory("unsupported-beta")

	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.SetToken("telegram-token")
	cfg.Channels.Discord.Enabled = true
	cfg.Channels.Discord.Token = *config.NewSecureString("discord-token")

	m, err := NewManager(cfg, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	got := m.GetEnabledChannels()
	want := map[string]bool{
		"telegram": true,
		"discord":  true,
	}
	if len(got) != len(want) {
		t.Fatalf("GetEnabledChannels() = %v, want %d channels", got, len(want))
	}
	for _, name := range got {
		if !want[name] {
			t.Fatalf("GetEnabledChannels() unexpectedly included %q", name)
		}
	}

	if _, ok := m.GetChannel("unsupported-alpha"); ok {
		t.Fatal("expected unsupported-alpha to be omitted from the runtime manager")
	}
	if _, ok := m.GetChannel("unsupported-beta"); ok {
		t.Fatal("expected unsupported-beta to be omitted from the runtime manager")
	}
}

func TestNewManager_OnlyInitializesTelegramAndDiscord_StartsCleanly(t *testing.T) {
	factoriesMu.Lock()
	originalFactories := make(map[string]ChannelFactory, len(factories))
	for name, factory := range factories {
		originalFactories[name] = factory
	}
	factories = map[string]ChannelFactory{}
	factoriesMu.Unlock()
	t.Cleanup(func() {
		factoriesMu.Lock()
		factories = originalFactories
		factoriesMu.Unlock()
	})

	registerStubFactory := func(name string) {
		RegisterFactory(name, func(cfg *config.Config, b *bus.MessageBus) (Channel, error) {
			return &mockChannel{
				BaseChannel: *NewBaseChannel(name, nil, b, nil),
			}, nil
		})
	}

	registerStubFactory("telegram")
	registerStubFactory("discord")
	registerStubFactory("unsupported-alpha")
	registerStubFactory("unsupported-beta")

	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.SetToken("telegram-token")
	cfg.Channels.Discord.Enabled = true
	cfg.Channels.Discord.Token = *config.NewSecureString("discord-token")

	m, err := NewManager(cfg, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}
	t.Cleanup(func() {
		_ = m.StopAll(context.Background())
	})
	if len(m.workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(m.workers))
	}
}

func TestNewManager_GetStartupStatusesReportsMissingToken(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true

	m, err := NewManager(cfg, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	statuses := startupStatusByName(m.GetStartupStatuses())

	telegram, ok := statuses["telegram"]
	if !ok {
		t.Fatalf("telegram status missing from startup report: %#v", statuses)
	}
	if telegram.State != ChannelStartupStateBlocked {
		t.Fatalf("telegram state = %q, want %q", telegram.State, ChannelStartupStateBlocked)
	}
	if telegram.Reason != "enabled but token missing" {
		t.Fatalf("telegram reason = %q, want %q", telegram.Reason, "enabled but token missing")
	}

	discord, ok := statuses["discord"]
	if !ok {
		t.Fatalf("discord status missing from startup report: %#v", statuses)
	}
	if discord.State != ChannelStartupStateDisabled {
		t.Fatalf("discord state = %q, want %q", discord.State, ChannelStartupStateDisabled)
	}
	if discord.Reason != "disabled in config" {
		t.Fatalf("discord reason = %q, want %q", discord.Reason, "disabled in config")
	}
}

func TestNewManager_GetStartupStatusesReportsDisabledChannels(t *testing.T) {
	cfg := config.DefaultConfig()

	m, err := NewManager(cfg, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	statuses := startupStatusByName(m.GetStartupStatuses())

	for _, name := range []string{"telegram", "discord"} {
		status, ok := statuses[name]
		if !ok {
			t.Fatalf("%s status missing from startup report: %#v", name, statuses)
		}
		if status.State != ChannelStartupStateDisabled {
			t.Fatalf("%s state = %q, want %q", name, status.State, ChannelStartupStateDisabled)
		}
		if status.Reason != "disabled in config" {
			t.Fatalf("%s reason = %q, want %q", name, status.Reason, "disabled in config")
		}
	}
}

func startupStatusByName(statuses []ChannelStartupStatus) map[string]ChannelStartupStatus {
	byName := make(map[string]ChannelStartupStatus, len(statuses))
	for _, status := range statuses {
		byName[status.Name] = status
	}
	return byName
}
