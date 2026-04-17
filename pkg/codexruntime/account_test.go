package codexruntime

import (
	"testing"
	"time"
)

func TestParseAccountReadResult_HandlesCurrentAppServerShape(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.April, 17, 0, 0, 0, 0, time.UTC)
	account, err := parseAccountReadResult(map[string]any{
		"account": map[string]any{
			"type":     "chatgpt",
			"email":    "user@example.com",
			"planType": "team",
		},
		"requiresOpenaiAuth": true,
	}, observedAt)
	if err != nil {
		t.Fatalf("parseAccountReadResult() error = %v", err)
	}
	if account.Email != "user@example.com" {
		t.Fatalf("Email = %q, want %q", account.Email, "user@example.com")
	}
	if account.PlanType != "team" {
		t.Fatalf("PlanType = %q, want %q", account.PlanType, "team")
	}
	if account.AuthMode != "chatgpt" {
		t.Fatalf("AuthMode = %q, want %q", account.AuthMode, "chatgpt")
	}
	if !account.ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt = %v, want %v", account.ObservedAt, observedAt)
	}
}

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

func TestParseRateLimitsResult_HandlesCurrentAppServerShape(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.April, 17, 0, 30, 0, 0, time.UTC)
	limits, err := parseRateLimitsResult(map[string]any{
		"rateLimits": map[string]any{
			"limitId":   "codex",
			"limitName": "codex",
			"primary": map[string]any{
				"usedPercent": 25,
				"resetsAt":    1776393900,
			},
		},
		"rateLimitsByLimitId": map[string]any{
			"codex": map[string]any{
				"limitId":   "codex",
				"limitName": "codex",
				"primary": map[string]any{
					"usedPercent": 25,
					"resetsAt":    1776393900,
				},
			},
			"codex_other": map[string]any{
				"limitId":   "codex_other",
				"limitName": "codex_other",
				"primary": map[string]any{
					"usedPercent": 42,
					"resetsAt":    1776397500,
				},
			},
		},
	}, observedAt)
	if err != nil {
		t.Fatalf("parseRateLimitsResult() error = %v", err)
	}
	if len(limits) != 2 {
		t.Fatalf("len(limits) = %d, want %d", len(limits), 2)
	}
	if limits[0].ID != "codex" {
		t.Fatalf("limits[0].ID = %q, want %q", limits[0].ID, "codex")
	}
	if limits[0].PrimaryUsedPercent == nil || *limits[0].PrimaryUsedPercent != 25 {
		t.Fatalf("limits[0].PrimaryUsedPercent = %#v, want 25", limits[0].PrimaryUsedPercent)
	}
	if limits[1].ID != "codex_other" {
		t.Fatalf("limits[1].ID = %q, want %q", limits[1].ID, "codex_other")
	}
	if limits[1].PrimaryUsedPercent == nil || *limits[1].PrimaryUsedPercent != 42 {
		t.Fatalf("limits[1].PrimaryUsedPercent = %#v, want 42", limits[1].PrimaryUsedPercent)
	}
}
