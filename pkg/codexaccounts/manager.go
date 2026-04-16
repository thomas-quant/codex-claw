package codexaccounts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

var ErrIsolationRequired = errors.New("isolated add required while runtime is active")

type LoginMode string

const (
	LoginModeBrowser    LoginMode = "browser"
	LoginModeDeviceAuth LoginMode = "device-auth"
)

type AddOptions struct {
	Isolated   bool
	DeviceAuth bool
}

type ImportOptions struct {
	AuthFile string
}

type ManagerOptions struct {
	RuntimeActive bool
	Login         func(context.Context, LoginMode, string) error
}

type AccountSummary struct {
	Alias   string
	Enabled bool
	Active  bool
}

type StatusSummary struct {
	TotalAccounts int
	EnabledCount  int
	ActiveAlias   string
}

type Manager struct {
	layout        Layout
	store         *Store
	runtimeActive bool
	login         func(context.Context, LoginMode, string) error
}

func NewManager(layout Layout, opts ManagerOptions) *Manager {
	login := opts.Login
	if login == nil {
		login = runCodexLogin
	}
	return &Manager{
		layout:        layout,
		store:         NewStore(layout),
		runtimeActive: opts.RuntimeActive,
		login:         login,
	}
}

func (m *Manager) Add(ctx context.Context, alias string, options AddOptions) error {
	if m.runtimeActive && !options.Isolated {
		return ErrIsolationRequired
	}

	state, err := m.store.LoadState()
	if err != nil {
		return err
	}
	if _, exists := state.Accounts[alias]; exists {
		return fmt.Errorf("account %q already exists", alias)
	}
	if _, err := m.store.ReadSnapshot(alias); err == nil {
		return fmt.Errorf("account %q already exists", alias)
	} else if !os.IsNotExist(err) {
		return err
	}

	mode := LoginModeBrowser
	if options.DeviceAuth {
		mode = LoginModeDeviceAuth
	}

	if options.Isolated {
		authBytes, err := m.captureIsolated(ctx, mode, alias)
		if err != nil {
			return err
		}
		return m.persistAccount(state, alias, authBytes)
	}

	backup, hadLiveAuth, err := m.readLiveAuth()
	if err != nil {
		return err
	}
	if state.ActiveAlias != "" && hadLiveAuth {
		if err := m.store.WriteSnapshot(state.ActiveAlias, backup); err != nil {
			return err
		}
	}
	if err := m.login(ctx, mode, m.layout.CodexHome); err != nil {
		_ = m.restoreLiveAuth(backup, hadLiveAuth)
		return err
	}

	captured, err := os.ReadFile(m.layout.LiveAuthFile)
	if err != nil {
		_ = m.restoreLiveAuth(backup, hadLiveAuth)
		if os.IsNotExist(err) {
			return fmt.Errorf("codex login did not leave auth.json behind")
		}
		return err
	}
	if err := m.persistAccount(state, alias, captured); err != nil {
		_ = m.restoreLiveAuth(backup, hadLiveAuth)
		return err
	}
	return m.restoreLiveAuth(backup, hadLiveAuth)
}

func (m *Manager) Import(_ context.Context, alias string, options ImportOptions) error {
	if options.AuthFile == "" {
		return fmt.Errorf("auth file is required")
	}

	state, err := m.store.LoadState()
	if err != nil {
		return err
	}
	if _, exists := state.Accounts[alias]; exists {
		return fmt.Errorf("account %q already exists", alias)
	}
	if _, err := m.store.ReadSnapshot(alias); err == nil {
		return fmt.Errorf("account %q already exists", alias)
	} else if !os.IsNotExist(err) {
		return err
	}

	payload, err := os.ReadFile(options.AuthFile)
	if err != nil {
		return err
	}

	backup, hadLiveAuth, err := m.readLiveAuth()
	if err != nil {
		return err
	}
	if state.ActiveAlias != "" && hadLiveAuth {
		if err := m.store.WriteSnapshot(state.ActiveAlias, backup); err != nil {
			return err
		}
	}

	if err := m.store.WriteSnapshot(alias, payload); err != nil {
		return err
	}
	if err := m.store.WriteLiveAuth(payload); err != nil {
		return err
	}

	state.ActiveAlias = alias
	state.Accounts[alias] = AccountState{Alias: alias, Enabled: true}
	return m.store.SaveState(state)
}

func (m *Manager) List(context.Context) ([]AccountSummary, error) {
	state, err := m.store.LoadState()
	if err != nil {
		return nil, err
	}

	aliases := make([]string, 0, len(state.Accounts))
	for alias := range state.Accounts {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	out := make([]AccountSummary, 0, len(aliases))
	for _, alias := range aliases {
		account := state.Accounts[alias]
		out = append(out, AccountSummary{
			Alias:   alias,
			Enabled: account.Enabled,
			Active:  alias == state.ActiveAlias,
		})
	}
	return out, nil
}

func (m *Manager) Status(ctx context.Context) (StatusSummary, error) {
	accounts, err := m.List(ctx)
	if err != nil {
		return StatusSummary{}, err
	}

	summary := StatusSummary{TotalAccounts: len(accounts)}
	for _, account := range accounts {
		if account.Enabled {
			summary.EnabledCount++
		}
		if account.Active {
			summary.ActiveAlias = account.Alias
		}
	}
	return summary, nil
}

func (m *Manager) Remove(ctx context.Context, alias string) error {
	state, err := m.store.LoadState()
	if err != nil {
		return err
	}
	if alias == state.ActiveAlias {
		return fmt.Errorf("cannot remove active account %q", alias)
	}
	delete(state.Accounts, alias)
	if err := m.store.SaveState(state); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(m.layout.SnapshotsDir, alias+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *Manager) Enable(ctx context.Context, alias string) error {
	return m.setEnabled(alias, true)
}

func (m *Manager) Disable(ctx context.Context, alias string) error {
	return m.setEnabled(alias, false)
}

func (m *Manager) setEnabled(alias string, enabled bool) error {
	state, err := m.store.LoadState()
	if err != nil {
		return err
	}
	account, ok := state.Accounts[alias]
	if !ok {
		if _, err := m.store.ReadSnapshot(alias); err != nil {
			return fmt.Errorf("account %q not found", alias)
		}
		account = AccountState{Alias: alias}
	}
	account.Alias = alias
	account.Enabled = enabled
	state.Accounts[alias] = account
	return m.store.SaveState(state)
}

func (m *Manager) captureIsolated(ctx context.Context, mode LoginMode, alias string) ([]byte, error) {
	if err := os.MkdirAll(m.layout.IsolatedHomesDir, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(m.layout.IsolatedHomesDir, alias+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	codexHome := filepath.Join(dir, "codex-home")
	if err := m.login(ctx, mode, codexHome); err != nil {
		return nil, err
	}
	authFile := filepath.Join(codexHome, "auth.json")
	payload, err := os.ReadFile(authFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("codex login did not leave auth.json behind")
		}
		return nil, err
	}
	return payload, nil
}

func (m *Manager) persistAccount(state State, alias string, authBytes []byte) error {
	if err := m.store.WriteSnapshot(alias, authBytes); err != nil {
		return err
	}
	state.Accounts[alias] = AccountState{Alias: alias, Enabled: true}
	return m.store.SaveState(state)
}

func (m *Manager) readLiveAuth() ([]byte, bool, error) {
	payload, err := os.ReadFile(m.layout.LiveAuthFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return payload, true, nil
}

func (m *Manager) restoreLiveAuth(payload []byte, exists bool) error {
	if exists {
		return m.store.WriteLiveAuth(payload)
	}
	if err := os.Remove(m.layout.LiveAuthFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func runCodexLogin(ctx context.Context, mode LoginMode, codexHome string) error {
	args := []string{"login"}
	if mode == LoginModeDeviceAuth {
		args = append(args, "--device-auth")
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex login failed: %w", err)
	}
	return nil
}
