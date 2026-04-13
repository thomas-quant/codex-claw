package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/codex-claw/pkg/providers"
)

const interactiveThreadInactivityLimit = 8 * time.Hour

func shouldForceFreshInteractiveThread(now time.Time, lastUserMessageAt time.Time) bool {
	if lastUserMessageAt.IsZero() {
		return false
	}
	return now.UTC().Sub(lastUserMessageAt.UTC()) > interactiveThreadInactivityLimit
}

func buildInteractiveBootstrapInput(messages []providers.Message, recentTurns int) string {
	if len(messages) == 0 {
		return ""
	}

	var b strings.Builder
	firstUser := len(messages)
	for i, msg := range messages {
		if isUserRole(msg.Role) {
			firstUser = i
			break
		}
	}

	for _, msg := range messages[:firstUser] {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", role, content)
	}

	if recentTurns <= 0 {
		return strings.TrimSpace(b.String())
	}

	start := findRecentTurnStart(messages, recentTurns)
	if start < 0 || start >= len(messages) {
		return strings.TrimSpace(b.String())
	}

	if start < firstUser {
		start = firstUser
	}

	for _, msg := range messages[start:] {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", role, content)
	}

	return strings.TrimSpace(b.String())
}

func findRecentTurnStart(messages []providers.Message, recentTurns int) int {
	if len(messages) == 0 {
		return 0
	}
	if recentTurns <= 0 {
		return len(messages)
	}

	boundaries := parseTurnBoundaries(messages)
	if len(boundaries) == 0 {
		return 0
	}

	if recentTurns >= len(boundaries) {
		return boundaries[0]
	}

	return boundaries[len(boundaries)-recentTurns]
}

func isUserRole(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "user")
}
