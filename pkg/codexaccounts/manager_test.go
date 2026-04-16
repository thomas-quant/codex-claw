package codexaccounts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManager_AddRequiresIsolationWhileRuntimeActive(t *testing.T) {
	t.Parallel()

	layout := ResolveLayout(t.TempDir())
	manager := NewManager(layout, ManagerOptions{
		RuntimeActive: true,
		Login: func(context.Context, LoginMode, string) error {
			t.Fatal("login should not run")
			return nil
		},
	})

	err := manager.Add(context.Background(), "alpha", AddOptions{})
	if !errors.Is(err, ErrIsolationRequired) {
		t.Fatalf("Add() error = %v, want ErrIsolationRequired", err)
	}
}

func TestManager_RemoveRejectsActiveAlias(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := ResolveLayout(root)
	store := NewStore(layout)
	if err := store.SaveState(State{
		Version:     1,
		ActiveAlias: "alpha",
		Accounts: map[string]AccountState{
			"alpha": {Alias: "alpha", Enabled: true},
			"beta":  {Alias: "beta", Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := store.WriteSnapshot("alpha", []byte(`{"alias":"alpha"}`)); err != nil {
		t.Fatalf("WriteSnapshot(alpha) error = %v", err)
	}
	if err := store.WriteSnapshot("beta", []byte(`{"alias":"beta"}`)); err != nil {
		t.Fatalf("WriteSnapshot(beta) error = %v", err)
	}

	manager := NewManager(layout, ManagerOptions{})
	err := manager.Remove(context.Background(), "alpha")
	if err == nil {
		t.Fatal("Remove() error = nil, want active alias rejection")
	}
	if _, statErr := os.Stat(filepath.Join(layout.SnapshotsDir, "alpha.json")); statErr != nil {
		t.Fatalf("active snapshot removed unexpectedly: %v", statErr)
	}
}

func TestManager_AddCapturesIsolatedSnapshot(t *testing.T) {
	t.Parallel()

	layout := ResolveLayout(t.TempDir())
	manager := NewManager(layout, ManagerOptions{
		RuntimeActive: true,
		Login: func(_ context.Context, mode LoginMode, codexHome string) error {
			if mode != LoginModeDeviceAuth {
				t.Fatalf("login mode = %q, want %q", mode, LoginModeDeviceAuth)
			}
			if err := os.MkdirAll(codexHome, 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"token":"alpha"}`), 0o600)
		},
	})

	if err := manager.Add(context.Background(), "alpha", AddOptions{Isolated: true, DeviceAuth: true}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(layout.SnapshotsDir, "alpha.json"))
	if err != nil {
		t.Fatalf("ReadFile(snapshot) error = %v", err)
	}
	if string(got) != `{"token":"alpha"}` {
		t.Fatalf("snapshot = %q, want %q", got, `{"token":"alpha"}`)
	}
}

func TestManager_ImportSeedsLiveAuthAndActiveAlias(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := ResolveLayout(root)
	store := NewStore(layout)
	if err := store.SaveState(State{
		Version:     1,
		ActiveAlias: "current",
		Accounts: map[string]AccountState{
			"current": {Alias: "current", Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := store.WriteSnapshot("current", []byte(`{"token":"stale-current"}`)); err != nil {
		t.Fatalf("WriteSnapshot(current) error = %v", err)
	}
	if err := store.WriteLiveAuth([]byte(`{"token":"live-current"}`)); err != nil {
		t.Fatalf("WriteLiveAuth(current) error = %v", err)
	}

	sourceDir := t.TempDir()
	sourceAuth := filepath.Join(sourceDir, "auth.json")
	if err := os.WriteFile(sourceAuth, []byte(`{"token":"imported"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(sourceAuth) error = %v", err)
	}

	manager := NewManager(layout, ManagerOptions{})
	if err := manager.Import(context.Background(), "imported", ImportOptions{AuthFile: sourceAuth}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	liveAuth, err := os.ReadFile(layout.LiveAuthFile)
	if err != nil {
		t.Fatalf("ReadFile(live auth) error = %v", err)
	}
	if string(liveAuth) != `{"token":"imported"}` {
		t.Fatalf("live auth = %q, want %q", liveAuth, `{"token":"imported"}`)
	}

	currentSnapshot, err := os.ReadFile(filepath.Join(layout.SnapshotsDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile(current snapshot) error = %v", err)
	}
	if string(currentSnapshot) != `{"token":"live-current"}` {
		t.Fatalf("current snapshot = %q, want %q", currentSnapshot, `{"token":"live-current"}`)
	}

	importedSnapshot, err := os.ReadFile(filepath.Join(layout.SnapshotsDir, "imported.json"))
	if err != nil {
		t.Fatalf("ReadFile(imported snapshot) error = %v", err)
	}
	if string(importedSnapshot) != `{"token":"imported"}` {
		t.Fatalf("imported snapshot = %q, want %q", importedSnapshot, `{"token":"imported"}`)
	}

	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.ActiveAlias != "imported" {
		t.Fatalf("ActiveAlias = %q, want imported", state.ActiveAlias)
	}
	if !state.Accounts["imported"].Enabled {
		t.Fatalf("imported account state = %+v, want enabled", state.Accounts["imported"])
	}
}
