package channels

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/thomas-quant/codex-claw/pkg/config"
	"github.com/thomas-quant/codex-claw/pkg/logger"
)

func TestToChannelHashes(t *testing.T) {
	logger.SetLevel(logger.DEBUG)
	cfg := config.DefaultConfig()
	results := toChannelHashes(cfg)
	assert.Equal(t, 0, len(results))
	logger.Debugf("results: %v", results)
	cfg2 := config.DefaultConfig()
	cfg2.Channels.Telegram.Enabled = true
	cfg2.Channels.Telegram.SetToken("telegram-1")
	results2 := toChannelHashes(cfg2)
	assert.Equal(t, 1, len(results2))
	logger.Debugf("results2: %v", results2)
	added, removed := compareChannels(results, results2)
	assert.ElementsMatch(t, []string{"telegram"}, added)
	assert.Empty(t, removed)
	cfg3 := config.DefaultConfig()
	cfg3.Channels.Telegram.Enabled = true
	cfg3.Channels.Telegram.SetToken("telegram-2")
	cfg3.Channels.Discord.Enabled = true
	cfg3.Channels.Discord.Token = *config.NewSecureString("discord-1")
	results3 := toChannelHashes(cfg3)
	assert.Equal(t, 2, len(results3))
	logger.Debugf("results3: %v", results3)
	added, removed = compareChannels(results2, results3)
	assert.ElementsMatch(t, []string{"discord", "telegram"}, added)
	assert.ElementsMatch(t, []string{"telegram"}, removed)
	cfg3.Channels.Telegram.SetToken("telegram-3")
	results4 := toChannelHashes(cfg3)
	assert.Equal(t, 2, len(results4))
	logger.Debugf("results4: %v", results4)
	added, removed = compareChannels(results3, results4)
	assert.ElementsMatch(t, []string{"telegram"}, removed)
	assert.ElementsMatch(t, []string{"telegram"}, added)
	cc, err := toChannelConfig(cfg3, added)
	assert.NoError(t, err)
	logger.Debugf("cc: %#v", cc.Telegram)
	assert.Equal(t, "telegram-3", cc.Telegram.Token.String())
	assert.Equal(t, true, cc.Telegram.Enabled)
	cc, err = toChannelConfig(cfg2, added)
	assert.NoError(t, err)
	logger.Debugf("cc: %#v", cc.Telegram)
	assert.Equal(t, "telegram-1", cc.Telegram.Token.String())
	assert.Equal(t, true, cc.Telegram.Enabled)
}
