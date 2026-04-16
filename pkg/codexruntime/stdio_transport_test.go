package codexruntime

import "testing"

func TestMergeCommandEnv_ReplacesAndAppends(t *testing.T) {
	t.Parallel()

	got := mergeCommandEnv([]string{"PATH=/bin", "HOME=/tmp/old"}, map[string]string{
		"HOME":       "/tmp/new",
		"CODEX_HOME": "/tmp/.codex-claw/codex-home",
	})
	if !containsEnv(got, "HOME=/tmp/new") {
		t.Fatalf("mergeCommandEnv() = %v, want replaced HOME", got)
	}
	if !containsEnv(got, "CODEX_HOME=/tmp/.codex-claw/codex-home") {
		t.Fatalf("mergeCommandEnv() = %v, want CODEX_HOME override", got)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
