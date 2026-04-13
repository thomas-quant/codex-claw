package discord

import (
	"github.com/thomas-quant/codex-claw/pkg/audio/tts"
	"github.com/thomas-quant/codex-claw/pkg/bus"
	"github.com/thomas-quant/codex-claw/pkg/channels"
	"github.com/thomas-quant/codex-claw/pkg/config"
)

func init() {
	channels.RegisterFactory("discord", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		ch, err := NewDiscordChannel(cfg.Channels.Discord, b)
		if err == nil {
			ch.tts = tts.DetectTTS(cfg)
		}
		return ch, err
	})
}
