package codexaccounts

import (
	"context"
	"errors"
	"testing"

	"github.com/thomas-quant/codex-claw/pkg/codexruntime"
)

func TestCoordinator_HardSwitchUpdatesActiveAliasBeforeRestart(t *testing.T) {
	t.Parallel()

	store := NewStore(ResolveLayout(t.TempDir()))
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
	if err := store.WriteSnapshot("alpha", []byte(`{"last_refresh":"2026-04-14T07:00:00Z"}`)); err != nil {
		t.Fatalf("WriteSnapshot(alpha) error = %v", err)
	}
	if err := store.WriteSnapshot("beta", []byte(`{"last_refresh":"2026-04-14T08:00:00Z"}`)); err != nil {
		t.Fatalf("WriteSnapshot(beta) error = %v", err)
	}

	runtime := &fakeCoordinatedRuntime{
		runErr: errors.New("usage limit reached"),
		rateLimits: []codexruntime.RateLimitSnapshot{
			{ID: "codex", Name: "Codex", PrimaryUsedPercent: intPtr(95), SecondaryUsedPercent: intPtr(30)},
		},
	}
	coord := NewCoordinator(runtime, store, CoordinatorOptions{
		IsUsageExhausted: func(err error) bool { return true },
	})

	_, err := coord.RunTextTurn(context.Background(), codexruntime.RunRequest{
		BindingKey: "telegram:chat-1:coder",
		Model:      "gpt-5.4",
		Input:      []codexruntime.TurnInputItem{{Type: "text", Text: "hello"}},
	})
	if err == nil {
		t.Fatal("RunTextTurn() error = nil, want hard-switch retry failure surfaced")
	}
	if len(runtime.runRequests) != 2 {
		t.Fatalf("RunTextTurn() attempts = %d, want 2 after hard switch retry", len(runtime.runRequests))
	}
	retry := runtime.runRequests[1]
	if !retry.Recovery.AllowResume || !retry.Recovery.AllowServerRestart {
		t.Fatalf("retry recovery = %#v, want resume+restart enabled", retry.Recovery)
	}

	state, loadErr := store.LoadState()
	if loadErr != nil {
		t.Fatalf("LoadState() error = %v", loadErr)
	}
	if state.ActiveAlias != "beta" {
		t.Fatalf("active alias = %q, want beta after swap", state.ActiveAlias)
	}

	liveAuth, readErr := store.ReadLiveAuth()
	if readErr != nil {
		t.Fatalf("ReadLiveAuth() error = %v", readErr)
	}
	if string(liveAuth) != `{"last_refresh":"2026-04-14T08:00:00Z"}` {
		t.Fatalf("live auth = %q, want beta snapshot", liveAuth)
	}
}

type fakeCoordinatedRuntime struct {
	runResult    codexruntime.RunResult
	runErr       error
	runRequests  []codexruntime.RunRequest
	rateLimits   []codexruntime.RateLimitSnapshot
	rateLimitErr error
	status       codexruntime.RuntimeStatusSnapshot
	statusErr    error
}

func (f *fakeCoordinatedRuntime) RunTextTurn(_ context.Context, req codexruntime.RunRequest) (codexruntime.RunResult, error) {
	f.runRequests = append(f.runRequests, req)
	if f.runErr != nil {
		return codexruntime.RunResult{}, f.runErr
	}
	return f.runResult, nil
}

func (f *fakeCoordinatedRuntime) CompactThread(context.Context, string) error {
	return nil
}

func (f *fakeCoordinatedRuntime) ListModels(context.Context) ([]codexruntime.ModelCatalogEntry, error) {
	return nil, nil
}

func (f *fakeCoordinatedRuntime) ReadStatus(context.Context, string) (codexruntime.RuntimeStatusSnapshot, error) {
	if f.statusErr != nil {
		return codexruntime.RuntimeStatusSnapshot{}, f.statusErr
	}
	return f.status, nil
}

func (f *fakeCoordinatedRuntime) SetModel(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeCoordinatedRuntime) SetThinkingMode(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeCoordinatedRuntime) ToggleFast(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeCoordinatedRuntime) ResetThread(context.Context, string) error {
	return nil
}

func (f *fakeCoordinatedRuntime) ReadAccount(context.Context, bool) (codexruntime.AccountSnapshot, error) {
	return codexruntime.AccountSnapshot{}, nil
}

func (f *fakeCoordinatedRuntime) ReadRateLimits(context.Context) ([]codexruntime.RateLimitSnapshot, error) {
	if f.rateLimitErr != nil {
		return nil, f.rateLimitErr
	}
	return append([]codexruntime.RateLimitSnapshot(nil), f.rateLimits...), nil
}

func (f *fakeCoordinatedRuntime) Close() error {
	return nil
}

func intPtr(v int) *int {
	return &v
}
