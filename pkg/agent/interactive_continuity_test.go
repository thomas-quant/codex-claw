package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/thomas-quant/codex-claw/pkg/providers"
)

func TestBuildInteractiveBootstrapInput_UsesRecentTurnsAndCurrentMessage(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", Content: "bootstrap"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply one"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "reply two"},
		{Role: "user", Content: "current"},
	}

	got := buildInteractiveBootstrapInput(messages, 2)
	want := "SYSTEM: system\nASSISTANT: bootstrap\nUSER: second\nASSISTANT: reply two\nUSER: current"
	if got != want {
		t.Fatalf("buildInteractiveBootstrapInput() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "SYSTEM: system") {
		t.Fatalf("buildInteractiveBootstrapInput() lost system context: %q", got)
	}
	if strings.Contains(got, "USER: first") {
		t.Fatalf("buildInteractiveBootstrapInput() kept too much history: %q", got)
	}
}

func TestBuildInteractiveBootstrapInput_ZeroRecentTurnsKeepsPrefix(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", Content: "bootstrap"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
	}

	got := buildInteractiveBootstrapInput(messages, 0)
	want := "SYSTEM: system\nASSISTANT: bootstrap"
	if got != want {
		t.Fatalf("buildInteractiveBootstrapInput() = %q, want %q", got, want)
	}
}

func TestBuildInteractiveBootstrapInput_NoUserKeepsContextPrefix(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "system"},
		{Role: "tool", Content: "tool result"},
		{Role: "assistant", Content: ""},
		{Role: "assistant", Content: "bootstrap"},
	}

	got := buildInteractiveBootstrapInput(messages, 2)
	want := "SYSTEM: system\nTOOL: tool result\nASSISTANT: bootstrap"
	if got != want {
		t.Fatalf("buildInteractiveBootstrapInput() = %q, want %q", got, want)
	}
}

func TestBuildInteractiveBootstrapInput_NormalizesRoleFormatting(t *testing.T) {
	messages := []providers.Message{
		{Role: "  SYSTEM ", Content: "system"},
		{Role: "Assistant", Content: "bootstrap"},
		{Role: "  User  ", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "USER", Content: "current"},
	}

	got := buildInteractiveBootstrapInput(messages, 1)
	want := "SYSTEM: system\nASSISTANT: bootstrap\nUSER: current"
	if got != want {
		t.Fatalf("buildInteractiveBootstrapInput() = %q, want %q", got, want)
	}
}

func TestBuildInteractiveRecoveryBootstrapInput_UsesLastFiveRawTurns(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "q3"},
		{Role: "assistant", Content: "a3"},
		{Role: "user", Content: "q4"},
		{Role: "assistant", Content: "a4"},
		{Role: "user", Content: "q5"},
		{Role: "assistant", Content: "a5"},
		{Role: "user", Content: "q6"},
	}

	got := buildInteractiveRecoveryBootstrapInput(history, 5)
	want := "USER: q2\nASSISTANT: a2\nUSER: q3\nASSISTANT: a3\nUSER: q4\nASSISTANT: a4\nUSER: q5\nASSISTANT: a5\nUSER: q6"
	if got != want {
		t.Fatalf("buildInteractiveRecoveryBootstrapInput() = %q, want %q", got, want)
	}
	if strings.Contains(got, "q1") || strings.Contains(got, "a1") {
		t.Fatalf("buildInteractiveRecoveryBootstrapInput() kept older raw turns: %q", got)
	}
}

func TestShouldForceFreshInteractiveThread_WhenInactiveForOverEightHours(t *testing.T) {
	now := time.Date(2026, time.April, 13, 12, 0, 0, 0, time.UTC)
	lastUserMessageAt := now.Add(-9 * time.Hour)

	if !shouldForceFreshInteractiveThread(now, lastUserMessageAt) {
		t.Fatal("shouldForceFreshInteractiveThread() = false, want true")
	}
}

func TestShouldForceFreshInteractiveThread_StaysFalseForWarmThread(t *testing.T) {
	now := time.Date(2026, time.April, 13, 12, 0, 0, 0, time.UTC)
	lastUserMessageAt := now.Add(-2 * time.Hour)

	if shouldForceFreshInteractiveThread(now, lastUserMessageAt) {
		t.Fatal("shouldForceFreshInteractiveThread() = true, want false")
	}
}

func TestShouldForceFreshInteractiveThread_ZeroTimestampStaysFalse(t *testing.T) {
	now := time.Date(2026, time.April, 13, 12, 0, 0, 0, time.UTC)

	if shouldForceFreshInteractiveThread(now, time.Time{}) {
		t.Fatal("shouldForceFreshInteractiveThread() = true, want false")
	}
}

func TestShouldForceFreshInteractiveThread_ExactBoundaryStaysFalse(t *testing.T) {
	now := time.Date(2026, time.April, 13, 12, 0, 0, 0, time.UTC)
	lastUserMessageAt := now.Add(-interactiveThreadInactivityLimit)

	if shouldForceFreshInteractiveThread(now, lastUserMessageAt) {
		t.Fatal("shouldForceFreshInteractiveThread() = true, want false at exact 8h boundary")
	}
}
