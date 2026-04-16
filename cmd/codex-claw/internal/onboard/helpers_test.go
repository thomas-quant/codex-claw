package onboard

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/config"
)

func TestCopyEmbeddedToTargetUsesStructuredAgentFiles(t *testing.T) {
	targetDir := t.TempDir()

	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}

	agentPath := filepath.Join(targetDir, "AGENT.md")
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("expected %s to exist: %v", agentPath, err)
	}

	soulPath := filepath.Join(targetDir, "SOUL.md")
	if _, err := os.Stat(soulPath); err != nil {
		t.Fatalf("expected %s to exist: %v", soulPath, err)
	}

	userPath := filepath.Join(targetDir, "USER.md")
	if _, err := os.Stat(userPath); err != nil {
		t.Fatalf("expected %s to exist: %v", userPath, err)
	}

	for _, legacyName := range []string{"AGENTS.md", "IDENTITY.md"} {
		legacyPath := filepath.Join(targetDir, legacyName)
		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			t.Fatalf("expected legacy file %s to be absent, got err=%v", legacyPath, err)
		}
	}
}

func TestChooseInitialSurface_DefaultsToTelegram(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	got, err := chooseInitialSurface(bytes.NewBufferString("\n"), &out)
	if err != nil {
		t.Fatalf("chooseInitialSurface() error = %v", err)
	}
	if got != surfaceTelegram {
		t.Fatalf("surface = %q, want %q", got, surfaceTelegram)
	}
	if out.Len() == 0 {
		t.Fatal("prompt output empty, want guidance text")
	}
}

func TestApplyInitialSurfaceSelection_EnablesOnlyChosenChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		surface       chatSurface
		wantTelegram  bool
		wantDiscord   bool
	}{
		{name: "telegram", surface: surfaceTelegram, wantTelegram: true, wantDiscord: false},
		{name: "discord", surface: surfaceDiscord, wantTelegram: false, wantDiscord: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()

			applyInitialSurfaceSelection(cfg, tt.surface)

			if cfg.Channels.Telegram.Enabled != tt.wantTelegram {
				t.Fatalf("Telegram.Enabled = %v, want %v", cfg.Channels.Telegram.Enabled, tt.wantTelegram)
			}
			if cfg.Channels.Discord.Enabled != tt.wantDiscord {
				t.Fatalf("Discord.Enabled = %v, want %v", cfg.Channels.Discord.Enabled, tt.wantDiscord)
			}

			var allowFrom []string
			if tt.surface == surfaceTelegram {
				allowFrom = []string(cfg.Channels.Telegram.AllowFrom)
			} else {
				allowFrom = []string(cfg.Channels.Discord.AllowFrom)
			}
			if len(allowFrom) != 1 || allowFrom[0] != "YOUR_USER_ID" {
				t.Fatalf("allow_from = %v, want [YOUR_USER_ID]", allowFrom)
			}
		})
	}
}

func TestMaybeImportLiveAuth_ImportsWhenConfirmed(t *testing.T) {
	t.Parallel()

	sourceAuth := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(sourceAuth, []byte(`{"token":"imported"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(sourceAuth) error = %v", err)
	}

	var importedPath string
	var out bytes.Buffer
	imported, err := maybeImportLiveAuth(bytes.NewBufferString("y\n"), &out, sourceAuth, func(path string) error {
		importedPath = path
		return nil
	})
	if err != nil {
		t.Fatalf("maybeImportLiveAuth() error = %v", err)
	}
	if !imported {
		t.Fatal("imported = false, want true")
	}
	if importedPath != sourceAuth {
		t.Fatalf("import path = %q, want %q", importedPath, sourceAuth)
	}
}
