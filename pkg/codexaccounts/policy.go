package codexaccounts

import "time"

const (
	softSwitchFiveHourPercent = 10
	weeklyFloorPercent        = 20
	telemetryFreshnessWindow  = 15 * time.Minute
	syncAfterRefreshAge       = 6 * time.Hour
)

func ShouldSoftSwitch(now time.Time, health HealthSnapshot) bool {
	if health.ObservedAt.IsZero() || now.UTC().Sub(health.ObservedAt.UTC()) > telemetryFreshnessWindow {
		return false
	}
	return health.FiveHourRemainingPct <= softSwitchFiveHourPercent
}

func ChooseTarget(activeAlias string, candidates []HealthSnapshot) (string, string, bool) {
	bestHealthy := ""
	bestHealthyFiveHour := -1
	bestHealthyWeekly := -1
	bestFallback := ""
	bestFallbackFiveHour := -1
	bestFallbackWeekly := -1

	for _, candidate := range candidates {
		if candidate.Alias == activeAlias || candidate.Status == "exhausted" || candidate.Status == "unknown" {
			continue
		}

		if candidate.WeeklyRemainingPct >= weeklyFloorPercent {
			if candidate.FiveHourRemainingPct > bestHealthyFiveHour ||
				(candidate.FiveHourRemainingPct == bestHealthyFiveHour && candidate.WeeklyRemainingPct > bestHealthyWeekly) {
				bestHealthy = candidate.Alias
				bestHealthyFiveHour = candidate.FiveHourRemainingPct
				bestHealthyWeekly = candidate.WeeklyRemainingPct
			}
			continue
		}

		if candidate.FiveHourRemainingPct > bestFallbackFiveHour ||
			(candidate.FiveHourRemainingPct == bestFallbackFiveHour && candidate.WeeklyRemainingPct > bestFallbackWeekly) {
			bestFallback = candidate.Alias
			bestFallbackFiveHour = candidate.FiveHourRemainingPct
			bestFallbackWeekly = candidate.WeeklyRemainingPct
		}
	}

	if bestHealthy != "" {
		return bestHealthy, "best_5h_headroom", true
	}
	if bestFallback != "" {
		return bestFallback, "least_bad_fallback", true
	}
	return "", "", false
}

func ShouldSyncAfterTurn(lastRefresh, now time.Time) bool {
	if lastRefresh.IsZero() {
		return false
	}
	return now.UTC().Sub(lastRefresh.UTC()) >= syncAfterRefreshAge
}
