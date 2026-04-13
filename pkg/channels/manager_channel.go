package channels

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

var survivingChannelNames = map[string]struct{}{
	"telegram": {},
	"discord":  {},
}

func toChannelHashes(cfg *config.Config) map[string]string {
	result := make(map[string]string)
	ch := cfg.Channels
	// should not be error
	marshal, _ := json.Marshal(ch)
	var channelConfig map[string]map[string]any
	_ = json.Unmarshal(marshal, &channelConfig)

	for key, value := range channelConfig {
		if _, ok := survivingChannelNames[key]; !ok {
			continue
		}
		if !value["enabled"].(bool) {
			continue
		}
		hiddenValues(key, value, ch)
		valueBytes, _ := json.Marshal(value)
		hash := md5.Sum(valueBytes)
		result[key] = hex.EncodeToString(hash[:])
	}

	return result
}

func hiddenValues(key string, value map[string]any, ch config.ChannelsConfig) {
	switch key {
	case "telegram":
		value["token"] = ch.Telegram.Token.String()
	case "discord":
		value["token"] = ch.Discord.Token.String()
	}
}

func compareChannels(old, news map[string]string) (added, removed []string) {
	for key, newHash := range news {
		if oldHash, ok := old[key]; ok {
			if newHash != oldHash {
				removed = append(removed, key)
				added = append(added, key)
			}
		} else {
			added = append(added, key)
		}
	}
	for key := range old {
		if _, ok := news[key]; !ok {
			removed = append(removed, key)
		}
	}
	return added, removed
}

func toChannelConfig(cfg *config.Config, list []string) (*config.ChannelsConfig, error) {
	result := &config.ChannelsConfig{}
	ch := cfg.Channels
	// should not be error
	marshal, _ := json.Marshal(ch)
	var channelConfig map[string]map[string]any
	_ = json.Unmarshal(marshal, &channelConfig)
	temp := make(map[string]map[string]any, 0)

	for key, value := range channelConfig {
		if _, ok := survivingChannelNames[key]; !ok {
			continue
		}
		found := false
		for _, s := range list {
			if key == s {
				found = true
				break
			}
		}
		if !found || !value["enabled"].(bool) {
			continue
		}
		temp[key] = value
	}

	marshal, err := json.Marshal(temp)
	if err != nil {
		logger.Errorf("marshal error: %v", err)
		return nil, err
	}
	err = json.Unmarshal(marshal, result)
	if err != nil {
		logger.Errorf("unmarshal error: %v", err)
		return nil, err
	}

	updateKeys(result, &ch)

	return result, nil
}

func updateKeys(newcfg, old *config.ChannelsConfig) {
	if newcfg.Telegram.Enabled {
		newcfg.Telegram.Token = old.Telegram.Token
	}
	if newcfg.Discord.Enabled {
		newcfg.Discord.Token = old.Discord.Token
	}
}
