package codexruntime

import (
	"encoding/json"
	"time"
)

func parseAccountReadResult(raw map[string]any, observedAt time.Time) (AccountSnapshot, error) {
	body, err := json.Marshal(raw)
	if err != nil {
		return AccountSnapshot{}, err
	}

	var payload struct {
		Result struct {
			Account struct {
				Email    string `json:"email"`
				PlanType string `json:"planType"`
				AuthMode string `json:"authMode"`
			} `json:"account"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountSnapshot{}, err
	}

	return AccountSnapshot{
		Email:      payload.Result.Account.Email,
		PlanType:   payload.Result.Account.PlanType,
		AuthMode:   payload.Result.Account.AuthMode,
		ObservedAt: observedAt,
	}, nil
}

func parseRateLimitsResult(raw map[string]any, observedAt time.Time) ([]RateLimitSnapshot, error) {
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Result struct {
			PlanType   string `json:"planType"`
			RateLimits []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Primary   window `json:"primary"`
				Secondary window `json:"secondary"`
			} `json:"rateLimits"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	limits := make([]RateLimitSnapshot, 0, len(payload.Result.RateLimits))
	for _, limit := range payload.Result.RateLimits {
		snapshot := RateLimitSnapshot{
			ID:         limit.ID,
			Name:       limit.Name,
			PlanType:   payload.Result.PlanType,
			ObservedAt: observedAt,
		}
		applyWindow(&snapshot.PrimaryUsedPercent, &snapshot.PrimaryResetAt, limit.Primary)
		applyWindow(&snapshot.SecondaryUsedPercent, &snapshot.SecondaryResetAt, limit.Secondary)
		limits = append(limits, snapshot)
	}

	return limits, nil
}

type window struct {
	UsedPercent int   `json:"usedPercent"`
	ResetsAt    int64 `json:"resetsAt"`
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
