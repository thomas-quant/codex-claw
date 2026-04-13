package asr

import (
	"testing"

	"github.com/sipeed/codex-claw/pkg/config"
)

func TestDetectTranscriber_NoConfigReturnsNil(t *testing.T) {
	if got := DetectTranscriber(nil); got != nil {
		t.Fatalf("DetectTranscriber(nil) = %v, want nil", got)
	}
}

func TestDetectTranscriber_CodexFirstRuntimeDisablesLegacyAutoDetection(t *testing.T) {
	cfg := &config.Config{
		Voice: config.VoiceConfig{ModelName: "whisper-1"},
	}

	if got := DetectTranscriber(cfg); got != nil {
		t.Fatalf("DetectTranscriber() = %v, want nil", got)
	}
}
