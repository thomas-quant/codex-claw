package codexruntime

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func parseAccountReadResult(raw map[string]any, observedAt time.Time) (AccountSnapshot, error) {
	body, err := json.Marshal(unwrapResultPayload(raw))
	if err != nil {
		return AccountSnapshot{}, err
	}

	var payload struct {
		Account *struct {
			Type     string `json:"type"`
			Email    string `json:"email"`
			PlanType string `json:"planType"`
			AuthMode string `json:"authMode"`
		} `json:"account"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountSnapshot{}, err
	}

	authMode := ""
	email := ""
	planType := ""
	if payload.Account != nil {
		authMode = normalizeAccountAuthMode(payload.Account.AuthMode, payload.Account.Type)
		email = payload.Account.Email
		planType = payload.Account.PlanType
	}

	return AccountSnapshot{
		Email:      email,
		PlanType:   planType,
		AuthMode:   authMode,
		ObservedAt: observedAt,
	}, nil
}

func parseRateLimitsResult(raw map[string]any, observedAt time.Time) ([]RateLimitSnapshot, error) {
	body, err := json.Marshal(unwrapResultPayload(raw))
	if err != nil {
		return nil, err
	}

	var payload struct {
		PlanType          string                  `json:"planType"`
		RateLimits        json.RawMessage         `json:"rateLimits"`
		RateLimitsByLimit map[string]rateLimitSet `json:"rateLimitsByLimitId"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	if len(payload.RateLimitsByLimit) > 0 {
		keys := make([]string, 0, len(payload.RateLimitsByLimit))
		for key := range payload.RateLimitsByLimit {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		limits := make([]RateLimitSnapshot, 0, len(keys))
		for _, key := range keys {
			limits = append(limits, payload.RateLimitsByLimit[key].snapshot(payload.PlanType, observedAt))
		}
		return limits, nil
	}

	if len(payload.RateLimits) == 0 {
		return nil, nil
	}

	var legacy []rateLimitSet
	if err := json.Unmarshal(payload.RateLimits, &legacy); err == nil {
		limits := make([]RateLimitSnapshot, 0, len(legacy))
		for _, limit := range legacy {
			limits = append(limits, limit.snapshot(payload.PlanType, observedAt))
		}
		return limits, nil
	}

	var single rateLimitSet
	if err := json.Unmarshal(payload.RateLimits, &single); err != nil {
		return nil, err
	}
	if single.empty() {
		return nil, nil
	}

	return []RateLimitSnapshot{single.snapshot(payload.PlanType, observedAt)}, nil
}

type window struct {
	UsedPercent int   `json:"usedPercent"`
	ResetsAt    int64 `json:"resetsAt"`
}

type rateLimitSet struct {
	ID        string `json:"id"`
	LimitID   string `json:"limitId"`
	Name      string `json:"name"`
	LimitName string `json:"limitName"`
	Primary   window `json:"primary"`
	Secondary window `json:"secondary"`
}

func (r rateLimitSet) empty() bool {
	return r.ID == "" &&
		r.LimitID == "" &&
		r.Name == "" &&
		r.LimitName == "" &&
		r.Primary == (window{}) &&
		r.Secondary == (window{})
}

func (r rateLimitSet) snapshot(planType string, observedAt time.Time) RateLimitSnapshot {
	snapshot := RateLimitSnapshot{
		ID:         firstNonEmptyString(r.LimitID, r.ID),
		Name:       firstNonEmptyString(r.LimitName, r.Name),
		PlanType:   planType,
		ObservedAt: observedAt,
	}
	applyWindow(&snapshot.PrimaryUsedPercent, &snapshot.PrimaryResetAt, r.Primary)
	applyWindow(&snapshot.SecondaryUsedPercent, &snapshot.SecondaryResetAt, r.Secondary)
	return snapshot
}

func applyWindow(used **int, resetAt *time.Time, value window) {
	if value.UsedPercent != 0 {
		percent := value.UsedPercent
		*used = &percent
	}
	if value.ResetsAt > 0 {
		*resetAt = time.Unix(value.ResetsAt, 0).UTC()
	}
}

func unwrapResultPayload(raw map[string]any) map[string]any {
	if result, ok := raw["result"].(map[string]any); ok {
		return result
	}
	return raw
}

func normalizeAccountAuthMode(authMode, accountType string) string {
	authMode = strings.TrimSpace(authMode)
	if authMode != "" {
		return authMode
	}

	switch strings.TrimSpace(accountType) {
	case "apiKey":
		return "apikey"
	default:
		return strings.TrimSpace(accountType)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
