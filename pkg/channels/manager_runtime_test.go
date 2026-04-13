package channels

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
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
	registerStubFactory("matrix")
	registerStubFactory("irc")

	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.SetToken("telegram-token")
	cfg.Channels.Discord.Enabled = true
	cfg.Channels.Discord.Token = *config.NewSecureString("discord-token")
	cfg.Channels.Matrix.Enabled = true
	cfg.Channels.Matrix.Homeserver = "https://matrix.example.com"
	cfg.Channels.Matrix.UserID = "@bot:example.com"
	cfg.Channels.Matrix.AccessToken.Set("matrix-token")
	cfg.Channels.IRC.Enabled = true
	cfg.Channels.IRC.Server = "irc.example.com:6697"
	cfg.Channels.IRC.Nick = "picoclaw"

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

	if _, ok := m.GetChannel("matrix"); ok {
		t.Fatal("expected matrix to be omitted from the runtime manager")
	}
	if _, ok := m.GetChannel("irc"); ok {
		t.Fatal("expected irc to be omitted from the runtime manager")
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
	registerStubFactory("matrix")
	registerStubFactory("irc")

	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.SetToken("telegram-token")
	cfg.Channels.Discord.Enabled = true
	cfg.Channels.Discord.Token = *config.NewSecureString("discord-token")
	cfg.Channels.Matrix.Enabled = true
	cfg.Channels.Matrix.Homeserver = "https://matrix.example.com"
	cfg.Channels.Matrix.UserID = "@bot:example.com"
	cfg.Channels.Matrix.AccessToken.Set("matrix-token")
	cfg.Channels.IRC.Enabled = true
	cfg.Channels.IRC.Server = "irc.example.com:6697"
	cfg.Channels.IRC.Nick = "picoclaw"

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
