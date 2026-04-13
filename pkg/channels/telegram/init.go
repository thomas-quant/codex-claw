package telegram

import (
	"github.com/sipeed/codex-claw/pkg/bus"
	"github.com/sipeed/codex-claw/pkg/channels"
	"github.com/sipeed/codex-claw/pkg/config"
)

func init() {
	channels.RegisterFactory("telegram", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewTelegramChannel(cfg, b)
	})
}
