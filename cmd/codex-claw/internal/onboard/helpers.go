package onboard

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/thomas-quant/codex-claw/cmd/codex-claw/internal"
	"github.com/thomas-quant/codex-claw/cmd/codex-claw/internal/cliui"
	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
	"github.com/thomas-quant/codex-claw/pkg/config"
	"github.com/thomas-quant/codex-claw/pkg/credential"
)

type chatSurface string

const (
	surfaceTelegram      chatSurface = "telegram"
	surfaceDiscord       chatSurface = "discord"
	defaultAllowFromUser             = "YOUR_USER_ID"
)

type onboardOptions struct {
	Encrypt        bool
	Surface        string
	ImportAuthFile string
}

func onboard(opts onboardOptions) {
	configPath := internal.GetConfigPath()

	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
		if opts.Encrypt {
			// Only ask for confirmation when *both* config and SSH key already exist,
			// indicating a full re-onboard that would reset the config to defaults.
			sshKeyPath, _ := credential.DefaultSSHKeyPath()
			if _, err := os.Stat(sshKeyPath); err == nil {
				// Both exist — confirm a full reset.
				fmt.Printf("Config already exists at %s\n", configPath)
				fmt.Print("Overwrite config with defaults? (y/n): ")
				var response string
				fmt.Scanln(&response)
				if response != "y" {
					fmt.Println("Aborted.")
					return
				}
				configExists = false // user agreed to reset; treat as fresh
			}
			// Config exists but SSH key is missing — keep existing config, only add SSH key.
		}
	}

	var err error
	if opts.Encrypt {
		fmt.Println("\nSet up credential encryption")
		fmt.Println("-----------------------------")
		passphrase, pErr := promptPassphrase()
		if pErr != nil {
			fmt.Printf("Error: %v\n", pErr)
			os.Exit(1)
		}
		// Expose the passphrase to credential.PassphraseProvider (which calls
		// os.Getenv by default) so that SaveConfig can encrypt api_keys.
		// This process is a one-shot CLI tool; the env var is never exposed outside
		// the current process and disappears when it exits.
		os.Setenv(credential.PassphraseEnvVar, passphrase)

		if err = setupSSHKey(); err != nil {
			fmt.Printf("Error generating SSH key: %v\n", err)
			os.Exit(1)
		}
	}

	var cfg *config.Config
	if configExists {
		// Preserve the existing config; SaveConfig will re-encrypt api_keys with the new passphrase.
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			fmt.Printf("Error loading existing config: %v\n", err)
			os.Exit(1)
		}
	} else {
		surface, surfaceErr := resolveInitialSurface(opts.Surface, os.Stdin, os.Stdout)
		if surfaceErr != nil {
			fmt.Printf("Error selecting chat surface: %v\n", surfaceErr)
			os.Exit(1)
		}
		cfg = config.DefaultConfig()
		applyInitialSurfaceSelection(cfg, surface)
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	sourceAuth := opts.ImportAuthFile
	if sourceAuth == "" {
		sourceAuth = defaultCodexAuthFile()
	}
	importedAuth, err := maybeImportLiveAuth(os.Stdin, os.Stdout, sourceAuth, importManagedLiveAuth)
	if err != nil {
		fmt.Printf("Error importing Codex auth: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	createWorkspaceTemplates(workspace)

	cliui.PrintOnboardComplete(internal.Logo, opts.Encrypt, configPath, selectedSurfaceLabel(cfg), importedAuth)
}

// promptPassphrase reads the encryption passphrase twice from the terminal
// (with echo disabled) and returns it. Returns an error if the passphrase is
// empty or if the two inputs do not match.
func promptPassphrase() (string, error) {
	fmt.Print("Enter passphrase for credential encryption: ")
	p1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}
	if len(p1) == 0 {
		return "", fmt.Errorf("passphrase must not be empty")
	}

	fmt.Print("Confirm passphrase: ")
	p2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("reading passphrase confirmation: %w", err)
	}

	if string(p1) != string(p2) {
		return "", fmt.Errorf("passphrases do not match")
	}
	return string(p1), nil
}

// setupSSHKey generates the codex-claw-specific SSH key at ~/.ssh/codex-claw_ed25519.key.
// If the key already exists the user is warned and asked to confirm overwrite.
// Answering anything other than "y" keeps the existing key (not an error).
func setupSSHKey() error {
	keyPath, err := credential.DefaultSSHKeyPath()
	if err != nil {
		return fmt.Errorf("cannot determine SSH key path: %w", err)
	}

	if _, err := os.Stat(keyPath); err == nil {
		fmt.Printf("\n⚠️  WARNING: %s already exists.\n", keyPath)
		fmt.Println("    Overwriting will invalidate any credentials previously encrypted with this key.")
		fmt.Print("    Overwrite? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" {
			fmt.Println("Keeping existing SSH key.")
			return nil
		}
	}

	if err := credential.GenerateSSHKey(keyPath); err != nil {
		return err
	}
	fmt.Printf("SSH key generated: %s\n", keyPath)
	return nil
}

func createWorkspaceTemplates(workspace string) {
	err := copyEmbeddedToTarget(workspace)
	if err != nil {
		fmt.Printf("Error copying workspace templates: %v\n", err)
	}
}

func chooseInitialSurface(in io.Reader, out io.Writer) (chatSurface, error) {
	_, _ = fmt.Fprint(out, "Initial chat surface [telegram/discord] (default telegram): ")

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return parseSurface(line)
}

func resolveInitialSurface(raw string, in io.Reader, out io.Writer) (chatSurface, error) {
	if strings.TrimSpace(raw) != "" {
		return parseSurface(raw)
	}
	return chooseInitialSurface(in, out)
}

func parseSurface(raw string) (chatSurface, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(surfaceTelegram):
		return surfaceTelegram, nil
	case string(surfaceDiscord):
		return surfaceDiscord, nil
	default:
		return "", fmt.Errorf("unsupported surface %q", strings.TrimSpace(raw))
	}
}

func applyInitialSurfaceSelection(cfg *config.Config, surface chatSurface) {
	switch surface {
	case surfaceDiscord:
		cfg.Channels.Telegram.Enabled = false
		cfg.Channels.Telegram.AllowFrom = nil
		cfg.Channels.Discord.Enabled = true
		cfg.Channels.Discord.AllowFrom = config.FlexibleStringSlice{defaultAllowFromUser}
	default:
		cfg.Channels.Telegram.Enabled = true
		cfg.Channels.Telegram.AllowFrom = config.FlexibleStringSlice{defaultAllowFromUser}
		cfg.Channels.Discord.Enabled = false
		cfg.Channels.Discord.AllowFrom = nil
	}
}

func maybeImportLiveAuth(in io.Reader, out io.Writer, sourceAuth string, importFn func(string) error) (bool, error) {
	if sourceAuth == "" {
		return false, nil
	}
	if _, err := os.Stat(sourceAuth); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	_, _ = fmt.Fprintf(out, "Import existing Codex auth from %s? (y/n): ", sourceAuth)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	if strings.ToLower(strings.TrimSpace(line)) != "y" {
		return false, nil
	}
	if err := importFn(sourceAuth); err != nil {
		return false, err
	}
	return true, nil
}

func importManagedLiveAuth(sourceAuth string) error {
	payload, err := os.ReadFile(sourceAuth)
	if err != nil {
		return err
	}
	layout := codexaccounts.ResolveLayout(internal.GetCodexClawHome())
	return codexaccounts.NewStore(layout).WriteLiveAuth(payload)
}

func defaultCodexAuthFile() string {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		codexHome = filepath.Join(homeDir, ".codex")
	}
	return filepath.Join(codexHome, "auth.json")
}

func selectedSurfaceLabel(cfg *config.Config) string {
	switch {
	case cfg.Channels.Telegram.Enabled:
		return string(surfaceTelegram)
	case cfg.Channels.Discord.Enabled:
		return string(surfaceDiscord)
	default:
		return ""
	}
}

func copyEmbeddedToTarget(targetDir string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("Failed to create target directory: %w", err)
	}

	// Walk through all files in embed.FS
	err := fs.WalkDir(embeddedFiles, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Read embedded file
		data, err := embeddedFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("Failed to read embedded file %s: %w", path, err)
		}

		new_path, err := filepath.Rel("workspace", path)
		if err != nil {
			return fmt.Errorf("Failed to get relative path for %s: %v\n", path, err)
		}

		// Build target file path
		targetPath := filepath.Join(targetDir, new_path)

		// Ensure target file's directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("Failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}

		// Write file
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			return fmt.Errorf("Failed to write file %s: %w", targetPath, err)
		}

		return nil
	})

	return err
}
