package agentsessions_test

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

func TestDetailPath_EscapesEachSegment(t *testing.T) {
	for _, tc := range []struct{ source, key, want string }{
		{"codex", "2025/10/01/rollout-a.jsonl", "/agent-sessions/codex/2025/10/01/rollout-a.jsonl"},
		{"claude-code/default", "-Users-me-proj/abc", "/agent-sessions/claude-code/default/-Users-me-proj/abc"},
		{"claude-code/work", "a b/c?d#e", "/agent-sessions/claude-code/work/a%20b/c%3Fd%23e"},
	} {
		if got := agentsessions.DetailPath(tc.source, tc.key); got != tc.want {
			t.Errorf("DetailPath(%q, %q) = %q, want %q", tc.source, tc.key, got, tc.want)
		}
	}
}
