package commands

import (
	"fmt"
	"strings"
	"time"
)

func runtimeSetModel(rt *Runtime, value string) (string, error) {
	if rt == nil {
		return "", nil
	}
	if rt.SetModel != nil {
		return rt.SetModel(value)
	}
	if rt.SwitchModel != nil {
		return rt.SwitchModel(value)
	}
	return "", nil
}

func runtimeSetThinking(rt *Runtime, value string) (string, error) {
	if rt == nil || rt.SetThinking == nil {
		return "", nil
	}
	return rt.SetThinking(value)
}

func runtimeToggleFast(rt *Runtime) (bool, error) {
	if rt == nil || rt.ToggleFast == nil {
		return false, nil
	}
	return rt.ToggleFast()
}

func runtimeCompactThread(rt *Runtime) error {
	if rt == nil || rt.CompactThread == nil {
		return nil
	}
	return rt.CompactThread()
}

func runtimeResetThread(rt *Runtime) error {
	if rt == nil || rt.ResetThread == nil {
		return nil
	}
	return rt.ResetThread()
}

func runtimeReadStatus(rt *Runtime) (StatusSnapshot, bool) {
	if rt == nil || rt.ReadStatus == nil {
		return StatusSnapshot{}, false
	}
	return rt.ReadStatus()
}

func runtimeListModels(rt *Runtime) ([]ModelInfo, bool) {
	if rt == nil || rt.ListModels == nil {
		return nil, false
	}
	return rt.ListModels(), true
}

func formatModelList(models []ModelInfo) string {
	if len(models) == 0 {
		return "No available models"
	}
	var b strings.Builder
	b.WriteString("Available Models:\n")
	for i, model := range models {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(model.Name)
		if model.Provider != "" {
			b.WriteString(" (")
			b.WriteString(model.Provider)
			b.WriteString(")")
		}
	}
	return b.String()
}

func formatStatusSnapshot(status StatusSnapshot) string {
	var lines []string
	if status.ThreadID != "" {
		lines = append(lines, fmt.Sprintf("Thread ID: %s", status.ThreadID))
	}
	if status.Model != "" {
		if status.Provider != "" {
			lines = append(lines, fmt.Sprintf("Model: %s (Provider: %s)", status.Model, status.Provider))
		} else {
			lines = append(lines, fmt.Sprintf("Model: %s", status.Model))
		}
	}
	if status.ThinkingMode != "" {
		lines = append(lines, fmt.Sprintf("Thinking: %s", status.ThinkingMode))
	}
	if status.FastEnabled {
		lines = append(lines, "Fast: enabled")
	} else {
		lines = append(lines, "Fast: disabled")
	}
	if status.RecoveryState != "" {
		lines = append(lines, fmt.Sprintf("Recovery state: %s", status.RecoveryState))
	}
	if hasAccountStatus(status) {
		if status.ActiveAccountAlias != "" {
			lines = append(lines, fmt.Sprintf("Active account: %s", status.ActiveAccountAlias))
		}
		if status.AccountHealth != "" {
			lines = append(lines, fmt.Sprintf("Account health: %s", status.AccountHealth))
		}
		lines = append(lines, fmt.Sprintf("Telemetry fresh: %s", yesNo(status.TelemetryFresh)))
		lines = append(lines, fmt.Sprintf("5h remaining: %d%%", status.FiveHourRemainingPct))
		lines = append(lines, fmt.Sprintf("Weekly remaining: %d%%", status.WeeklyRemainingPct))
		if status.SwitchTrigger != "" {
			lines = append(lines, fmt.Sprintf("Switch trigger: %s", status.SwitchTrigger))
		}
	}
	if !status.LastUserMessageAt.IsZero() {
		lines = append(lines, fmt.Sprintf("Last user message: %s", formatStatusTime(status.LastUserMessageAt)))
	}
	if !status.LastCompactionAt.IsZero() {
		lines = append(lines, fmt.Sprintf("Last compaction: %s", formatStatusTime(status.LastCompactionAt)))
	}
	if status.ForceFreshThread {
		lines = append(lines, "Force fresh thread: yes")
	}
	if len(lines) == 0 {
		return "No runtime status available"
	}
	return strings.Join(lines, "\n")
}

func formatStatusTime(ts time.Time) string {
	return ts.UTC().Format("2006-01-02 15:04 UTC")
}

func hasAccountStatus(status StatusSnapshot) bool {
	return status.ActiveAccountAlias != "" ||
		status.AccountHealth != "" ||
		status.TelemetryFresh ||
		status.FiveHourRemainingPct != 0 ||
		status.WeeklyRemainingPct != 0 ||
		status.SwitchTrigger != ""
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
