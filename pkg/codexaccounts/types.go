package codexaccounts

import "time"

type Layout struct {
	HomeRoot         string
	CodexHome        string
	LiveAuthFile     string
	AccountsRoot     string
	SnapshotsDir     string
	StateFile        string
	HealthFile       string
	SwitchAuditFile  string
	IsolatedHomesDir string
}

type AccountState struct {
	Alias   string `json:"alias"`
	Enabled bool   `json:"enabled"`
}

type State struct {
	Version     int                     `json:"version"`
	ActiveAlias string                  `json:"active_alias,omitempty"`
	Accounts    map[string]AccountState `json:"accounts,omitempty"`
}

type HealthSnapshot struct {
	Alias                string    `json:"alias"`
	Status               string    `json:"status"`
	FiveHourRemainingPct int       `json:"five_hour_remaining_pct,omitempty"`
	WeeklyRemainingPct   int       `json:"weekly_remaining_pct,omitempty"`
	FiveHourResetAt      time.Time `json:"five_hour_reset_at,omitempty"`
	WeeklyResetAt        time.Time `json:"weekly_reset_at,omitempty"`
	ObservedAt           time.Time `json:"observed_at,omitempty"`
}

type SwitchEvent struct {
	OccurredAt       time.Time `json:"occurred_at"`
	SourceAlias      string    `json:"source_alias,omitempty"`
	TargetAlias      string    `json:"target_alias,omitempty"`
	Trigger          string    `json:"trigger"`
	RouteReason      string    `json:"route_reason"`
	ResumeMode       string    `json:"resume_mode"`
	TelemetryFresh   bool      `json:"telemetry_fresh"`
	AppServerRestart bool      `json:"app_server_restart"`
}
