package codexruntime

import (
	"testing"
	"time"
)

func TestParseRateLimitsResult_HandlesPrimaryAndSecondaryWindows(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.April, 14, 12, 0, 0, 0, time.UTC)
	limits, err := parseRateLimitsResult(map[string]any{
		"result": map[string]any{
			"planType": "plus",
			"rateLimits": []map[string]any{
				{
					"id":   "codex",
					"name": "Codex",
					"primary": map[string]any{
						"usedPercent":        12,
						"windowDurationMins": 300,
						"resetsAt":           1776160800,
					},
					"secondary": map[string]any{
						"usedPercent":        43,
						"windowDurationMins": 10080,
						"resetsAt":           1776660000,
					},
				},
			},
		},
	}, observedAt)
	if err != nil {
		t.Fatalf("parseRateLimitsResult() error = %v", err)
	}
	if len(limits) != 1 || limits[0].PlanType != "plus" {
		t.Fatalf("parseRateLimitsResult() = %#v, want one plus limit", limits)
	}
	if limits[0].PrimaryUsedPercent == nil || *limits[0].PrimaryUsedPercent != 12 {
		t.Fatalf("primary used percent = %#v, want 12", limits[0].PrimaryUsedPercent)
	}
	if limits[0].SecondaryUsedPercent == nil || *limits[0].SecondaryUsedPercent != 43 {
		t.Fatalf("secondary used percent = %#v, want 43", limits[0].SecondaryUsedPercent)
	}
}
