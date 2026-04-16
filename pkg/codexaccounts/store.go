package codexaccounts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

type Store struct {
	layout Layout
}

func ResolveLayout(homeRoot string) Layout {
	return Layout{
		HomeRoot:         homeRoot,
		CodexHome:        filepath.Join(homeRoot, "codex-home"),
		LiveAuthFile:     filepath.Join(homeRoot, "codex-home", "auth.json"),
		AccountsRoot:     filepath.Join(homeRoot, "codex-accounts"),
		SnapshotsDir:     filepath.Join(homeRoot, "codex-accounts", "accounts"),
		StateFile:        filepath.Join(homeRoot, "codex-accounts", "state.json"),
		HealthFile:       filepath.Join(homeRoot, "codex-accounts", "health.json"),
		SwitchAuditFile:  filepath.Join(homeRoot, "codex-accounts", "switches.jsonl"),
		IsolatedHomesDir: filepath.Join(homeRoot, "isolated-homes"),
	}
}

func NewStore(layout Layout) *Store {
	return &Store{layout: layout}
}

func (s *Store) WriteSnapshot(alias string, payload []byte) error {
	if !aliasPattern.MatchString(alias) {
		return fmt.Errorf("invalid alias %q", alias)
	}
	if err := os.MkdirAll(s.layout.SnapshotsDir, 0o700); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.layout.SnapshotsDir, alias+".json"), payload, 0o600)
}

func (s *Store) ReadSnapshot(alias string) ([]byte, error) {
	if !aliasPattern.MatchString(alias) {
		return nil, fmt.Errorf("invalid alias %q", alias)
	}
	return os.ReadFile(filepath.Join(s.layout.SnapshotsDir, alias+".json"))
}

func (s *Store) SaveState(state State) error {
	if err := os.MkdirAll(s.layout.AccountsRoot, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.layout.StateFile, append(body, '\n'), 0o600)
}

func (s *Store) LoadState() (State, error) {
	raw, err := os.ReadFile(s.layout.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Version: 1, Accounts: map[string]AccountState{}}, nil
		}
		return State{}, err
	}

	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	if state.Accounts == nil {
		state.Accounts = map[string]AccountState{}
	}
	return state, nil
}

func (s *Store) SaveHealth(health map[string]HealthSnapshot) error {
	if err := os.MkdirAll(s.layout.AccountsRoot, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(health, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.layout.HealthFile, append(body, '\n'), 0o600)
}

func (s *Store) AppendSwitchEvent(event SwitchEvent) error {
	if err := os.MkdirAll(s.layout.AccountsRoot, 0o700); err != nil {
		return err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.layout.SwitchAuditFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(line, '\n'))
	return err
}

func (s *Store) ReadLiveAuth() ([]byte, error) {
	return os.ReadFile(s.layout.LiveAuthFile)
}

func (s *Store) WriteLiveAuth(payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.layout.LiveAuthFile), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(s.layout.LiveAuthFile, payload, 0o600)
}

func writeFileAtomic(path string, body []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
