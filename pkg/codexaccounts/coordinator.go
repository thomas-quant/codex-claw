package codexaccounts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thomas-quant/codex-claw/pkg/codexruntime"
)

type coordinatedRuntime interface {
	RunTextTurn(context.Context, codexruntime.RunRequest) (codexruntime.RunResult, error)
	CompactThread(context.Context, string) error
	ListModels(context.Context) ([]codexruntime.ModelCatalogEntry, error)
	ReadStatus(context.Context, string) (codexruntime.RuntimeStatusSnapshot, error)
	SetModel(context.Context, string, string) (string, error)
	SetThinkingMode(context.Context, string, string) (string, error)
	ToggleFast(context.Context, string) (bool, error)
	ResetThread(context.Context, string) error
	ReadRateLimits(context.Context) ([]codexruntime.RateLimitSnapshot, error)
	Close() error
}

type CoordinatorOptions struct {
	IsUsageExhausted func(error) bool
}

type Coordinator struct {
	mu      sync.Mutex
	runtime coordinatedRuntime
	store   *Store
	opts    CoordinatorOptions
	now     func() time.Time
}

func NewCoordinator(runtime coordinatedRuntime, store *Store, opts CoordinatorOptions) *Coordinator {
	return &Coordinator{
		runtime: runtime,
		store:   store,
		opts:    opts,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (c *Coordinator) RunTextTurn(ctx context.Context, req codexruntime.RunRequest) (codexruntime.RunResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switched, err := c.maybeSoftSwitchLocked(ctx)
	if err != nil {
		return codexruntime.RunResult{}, err
	}
	if switched {
		req = enableSwitchRecovery(req)
	}

	result, err := c.runtime.RunTextTurn(ctx, req)
	if err == nil {
		return result, c.syncLiveAuthLocked(ctx)
	}
	if c.opts.IsUsageExhausted == nil || !c.opts.IsUsageExhausted(err) {
		return codexruntime.RunResult{}, err
	}
	return c.retryAfterHardSwitchLocked(ctx, req, err)
}

func (c *Coordinator) CompactThread(ctx context.Context, bindingKey string) error {
	return c.runtime.CompactThread(ctx, bindingKey)
}

func (c *Coordinator) ListModels(ctx context.Context) ([]codexruntime.ModelCatalogEntry, error) {
	return c.runtime.ListModels(ctx)
}

func (c *Coordinator) ReadStatus(ctx context.Context, bindingKey string) (codexruntime.RuntimeStatusSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	status, err := c.runtime.ReadStatus(ctx, bindingKey)
	if err != nil {
		return codexruntime.RuntimeStatusSnapshot{}, err
	}

	state, err := c.store.LoadState()
	if err != nil {
		return codexruntime.RuntimeStatusSnapshot{}, err
	}
	if state.ActiveAlias != "" {
		status.ActiveAccountAlias = state.ActiveAlias
	}

	healthByAlias, err := c.loadHealthLocked()
	if err != nil {
		return codexruntime.RuntimeStatusSnapshot{}, err
	}
	if health, ok := healthByAlias[status.ActiveAccountAlias]; ok {
		status.AccountHealth = health.Status
		status.TelemetryFresh = isTelemetryFresh(c.now(), health)
		status.FiveHourRemainingPct = health.FiveHourRemainingPct
		status.WeeklyRemainingPct = health.WeeklyRemainingPct
	}
	if status.SwitchTrigger == "" {
		trigger, err := c.lastSwitchTriggerLocked()
		if err != nil {
			return codexruntime.RuntimeStatusSnapshot{}, err
		}
		status.SwitchTrigger = trigger
	}

	return status, nil
}

func (c *Coordinator) SetModel(ctx context.Context, bindingKey, model string) (string, error) {
	return c.runtime.SetModel(ctx, bindingKey, model)
}

func (c *Coordinator) SetThinkingMode(ctx context.Context, bindingKey, thinkingMode string) (string, error) {
	return c.runtime.SetThinkingMode(ctx, bindingKey, thinkingMode)
}

func (c *Coordinator) ToggleFast(ctx context.Context, bindingKey string) (bool, error) {
	return c.runtime.ToggleFast(ctx, bindingKey)
}

func (c *Coordinator) ResetThread(ctx context.Context, bindingKey string) error {
	return c.runtime.ResetThread(ctx, bindingKey)
}

func (c *Coordinator) ReadRateLimits(ctx context.Context) ([]codexruntime.RateLimitSnapshot, error) {
	return c.runtime.ReadRateLimits(ctx)
}

func (c *Coordinator) Close() error {
	return c.runtime.Close()
}

func (c *Coordinator) maybeSoftSwitchLocked(_ context.Context) (bool, error) {
	state, err := c.store.LoadState()
	if err != nil {
		return false, err
	}
	if state.ActiveAlias == "" {
		return false, nil
	}

	healthByAlias, err := c.loadHealthLocked()
	if err != nil {
		return false, err
	}
	activeHealth, ok := healthByAlias[state.ActiveAlias]
	if !ok || !ShouldSoftSwitch(c.now(), activeHealth) {
		return false, nil
	}

	target, reason, ok := ChooseTarget(state.ActiveAlias, enabledHealthCandidates(state, healthByAlias))
	if !ok {
		return false, nil
	}
	if err := c.switchActiveAliasLocked(state, target, "soft_threshold_5h", reason, true); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Coordinator) retryAfterHardSwitchLocked(ctx context.Context, req codexruntime.RunRequest, originalErr error) (codexruntime.RunResult, error) {
	state, err := c.store.LoadState()
	if err != nil {
		return codexruntime.RunResult{}, err
	}
	if state.ActiveAlias == "" {
		return codexruntime.RunResult{}, originalErr
	}

	healthByAlias, err := c.loadHealthLocked()
	if err != nil {
		return codexruntime.RunResult{}, err
	}
	if refreshErr := c.refreshHealthLocked(ctx, state.ActiveAlias, healthByAlias); refreshErr != nil {
		if !errors.Is(refreshErr, os.ErrNotExist) {
			return codexruntime.RunResult{}, refreshErr
		}
	}
	current := healthByAlias[state.ActiveAlias]
	if current.Alias == "" {
		current.Alias = state.ActiveAlias
	}
	current.Status = "exhausted"
	current.FiveHourRemainingPct = 0
	current.ObservedAt = c.now()
	healthByAlias[state.ActiveAlias] = current
	if err := c.store.SaveHealth(healthByAlias); err != nil {
		return codexruntime.RunResult{}, err
	}

	target, reason, ok := ChooseTarget(state.ActiveAlias, enabledHealthCandidates(state, healthByAlias))
	if !ok {
		target, ok = fallbackAlias(state, state.ActiveAlias)
		reason = "first_enabled_fallback"
	}
	if !ok {
		return codexruntime.RunResult{}, originalErr
	}
	if err := c.switchActiveAliasLocked(state, target, "hard_usage_exhausted", reason, true); err != nil {
		return codexruntime.RunResult{}, err
	}

	retry := enableSwitchRecovery(req)
	result, err := c.runtime.RunTextTurn(ctx, retry)
	if err != nil {
		return codexruntime.RunResult{}, err
	}
	return result, c.syncLiveAuthLocked(ctx)
}

func (c *Coordinator) syncLiveAuthLocked(ctx context.Context) error {
	state, err := c.store.LoadState()
	if err != nil {
		return err
	}
	if state.ActiveAlias == "" {
		return nil
	}

	healthByAlias, err := c.loadHealthLocked()
	if err != nil {
		return err
	}
	if err := c.refreshHealthLocked(ctx, state.ActiveAlias, healthByAlias); err != nil {
		return err
	}

	lastRefresh := c.snapshotRefreshTimeLocked(state.ActiveAlias)
	if !ShouldSyncAfterTurn(lastRefresh, c.now()) {
		return nil
	}

	liveAuth, err := c.store.ReadLiveAuth()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return c.store.WriteSnapshot(state.ActiveAlias, liveAuth)
}

func (c *Coordinator) refreshHealthLocked(ctx context.Context, activeAlias string, healthByAlias map[string]HealthSnapshot) error {
	limits, err := c.runtime.ReadRateLimits(ctx)
	if err != nil {
		return err
	}

	observedAt := c.now()
	snapshot := HealthSnapshot{
		Alias:      activeAlias,
		Status:     "healthy",
		ObservedAt: observedAt,
	}
	for _, limit := range limits {
		if limit.PrimaryUsedPercent != nil {
			snapshot.FiveHourRemainingPct = clampRemainingPercent(*limit.PrimaryUsedPercent)
			snapshot.FiveHourResetAt = limit.PrimaryResetAt
		}
		if limit.SecondaryUsedPercent != nil {
			snapshot.WeeklyRemainingPct = clampRemainingPercent(*limit.SecondaryUsedPercent)
			snapshot.WeeklyResetAt = limit.SecondaryResetAt
		}
	}
	switch {
	case snapshot.FiveHourRemainingPct <= 0 || snapshot.WeeklyRemainingPct <= 0:
		snapshot.Status = "exhausted"
	case snapshot.FiveHourRemainingPct <= softSwitchFiveHourPercent || snapshot.WeeklyRemainingPct < weeklyFloorPercent:
		snapshot.Status = "soft-drain"
	}
	if healthByAlias == nil {
		healthByAlias = map[string]HealthSnapshot{}
	}
	healthByAlias[activeAlias] = snapshot
	return c.store.SaveHealth(healthByAlias)
}

func (c *Coordinator) switchActiveAliasLocked(state State, targetAlias, trigger, routeReason string, appServerRestart bool) error {
	snapshot, err := c.store.ReadSnapshot(targetAlias)
	if err != nil {
		return err
	}
	if err := c.store.WriteLiveAuth(snapshot); err != nil {
		return err
	}

	sourceAlias := state.ActiveAlias
	state.ActiveAlias = targetAlias
	if err := c.store.SaveState(state); err != nil {
		return err
	}

	return c.store.AppendSwitchEvent(SwitchEvent{
		OccurredAt:       c.now(),
		SourceAlias:      sourceAlias,
		TargetAlias:      targetAlias,
		Trigger:          trigger,
		RouteReason:      routeReason,
		ResumeMode:       "same_thread_resume",
		TelemetryFresh:   true,
		AppServerRestart: appServerRestart,
	})
}

func (c *Coordinator) loadHealthLocked() (map[string]HealthSnapshot, error) {
	raw, err := os.ReadFile(c.store.layout.HealthFile)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]HealthSnapshot{}, nil
		}
		return nil, err
	}

	var health map[string]HealthSnapshot
	if err := json.Unmarshal(raw, &health); err != nil {
		return nil, err
	}
	if health == nil {
		health = map[string]HealthSnapshot{}
	}
	return health, nil
}

func (c *Coordinator) lastSwitchTriggerLocked() (string, error) {
	file, err := os.Open(c.store.layout.SwitchAuditFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()

	var lastLine string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lastLine = line
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if lastLine == "" {
		return "", nil
	}

	var event SwitchEvent
	if err := json.Unmarshal([]byte(lastLine), &event); err != nil {
		return "", err
	}
	return event.Trigger, nil
}

func (c *Coordinator) snapshotRefreshTimeLocked(alias string) time.Time {
	payload, err := c.store.ReadSnapshot(alias)
	if err == nil {
		var meta struct {
			LastRefresh string `json:"last_refresh"`
		}
		if json.Unmarshal(payload, &meta) == nil && meta.LastRefresh != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, meta.LastRefresh); parseErr == nil {
				return parsed.UTC()
			}
		}
	}

	info, err := os.Stat(c.store.layout.SnapshotsDir + string(os.PathSeparator) + alias + ".json")
	if err != nil {
		return time.Time{}
	}
	return info.ModTime().UTC()
}

func enableSwitchRecovery(req codexruntime.RunRequest) codexruntime.RunRequest {
	req.Recovery.AllowResume = true
	req.Recovery.AllowServerRestart = true
	return req
}

func enabledHealthCandidates(state State, healthByAlias map[string]HealthSnapshot) []HealthSnapshot {
	candidates := make([]HealthSnapshot, 0, len(state.Accounts))
	for alias, account := range state.Accounts {
		if !account.Enabled {
			continue
		}
		health, ok := healthByAlias[alias]
		if !ok {
			continue
		}
		candidates = append(candidates, health)
	}
	return candidates
}

func fallbackAlias(state State, exclude string) (string, bool) {
	aliases := make([]string, 0, len(state.Accounts))
	for alias := range state.Accounts {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		account := state.Accounts[alias]
		if alias == exclude || !account.Enabled {
			continue
		}
		return alias, true
	}
	return "", false
}

func isTelemetryFresh(now time.Time, health HealthSnapshot) bool {
	if health.ObservedAt.IsZero() {
		return false
	}
	return now.UTC().Sub(health.ObservedAt.UTC()) <= telemetryFreshnessWindow
}

func clampRemainingPercent(usedPercent int) int {
	switch {
	case usedPercent <= 0:
		return 100
	case usedPercent >= 100:
		return 0
	default:
		return 100 - usedPercent
	}
}
