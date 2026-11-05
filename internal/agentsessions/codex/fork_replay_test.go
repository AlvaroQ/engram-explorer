package codex_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

// TestListForkReplayDedupe pins the cross-file dedupe of L6.5 with fixtures
// from the independent verification: a fork that replays its parent's
// token_count rows (plus a re-emit and a null info) is credited only with its
// own usage, and token_usage_record rows repeated within and across files are
// counted once per response_id.
func TestListForkReplayDedupe(t *testing.T) {
	src := codex.New("codex", filepath.Join("testdata", "forkreplay"))
	ss, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]agentsessions.Usage{}
	for _, s := range ss {
		byID[s.ID] = s.Usage
	}
	type want struct{ input, cacheRead, cacheWrite, output, reasoning, total int64 }
	cases := map[string]want{
		"P1": {1400, 1600, 0, 400, 120, 3400},
		"F1": {1000, 1000, 0, 300, 80, 2300},
		"R1": {2200, 500, 300, 300, 50, 3300},
		"S1": {400, 100, 0, 50, 10, 550},
	}
	if len(byID) != len(cases) {
		t.Fatalf("listed %d sessions, want %d", len(byID), len(cases))
	}
	for id, w := range cases {
		u, ok := byID[id]
		if !ok {
			t.Errorf("session %s not listed", id)
			continue
		}
		got := want{u.Input, u.CacheRead, u.CacheWrite, u.Output, u.Reasoning, u.Total()}
		if got != w {
			t.Errorf("%s usage = %+v, want %+v", id, got, w)
		}
	}
}
