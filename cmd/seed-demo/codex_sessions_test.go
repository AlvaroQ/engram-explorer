package main

import (
	"context"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

// wantCodexUsage is the disjoint-bucket usage of each sample rollout, worked
// out by hand from the rollout data (cached tokens carved out of input,
// reasoning a subset of output, re-emitted token_count rows counted once).
var wantCodexUsage = map[string]agentsessions.Usage{
	"0199f3a2-5c41-7d10-9e2b-6a1c0d4e8f01": {Input: 17358, CacheRead: 14592, Output: 1480, Reasoning: 896},
	"0199f6b8-2d07-7a3e-8c15-3f9e7b2a4d02": {Input: 20392, CacheRead: 9728, CacheWrite: 1024, Output: 1665, Reasoning: 896},
	"0199f8e1-9b3c-7f24-a6d8-1e5c4a7b9f03": {Input: 7854, CacheRead: 6016, Output: 742, Reasoning: 448},
}

var wantCodexTotal = map[string]int64{
	"0199f3a2-5c41-7d10-9e2b-6a1c0d4e8f01": 33430,
	"0199f6b8-2d07-7a3e-8c15-3f9e7b2a4d02": 32809,
	"0199f8e1-9b3c-7f24-a6d8-1e5c4a7b9f03": 14612,
}

var rolloutKey = regexp.MustCompile(`^(\d{4})/(\d{2})/(\d{2})/rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-([0-9a-f-]{36})\.jsonl$`)

// TestRun_WritesSampleCodexSessions: the seeder writes sample Codex CLI
// rollouts under <out>/codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl
// that the Codex reader lists with exact disjoint usage priced by a known
// model, recent start times, head metadata and a paired tool call in detail.
func TestRun_WritesSampleCodexSessions(t *testing.T) {
	pinClock(t)
	out := filepath.Join(t.TempDir(), "demo")
	if err := run([]string{"--out", out}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	src := codex.New("codex", filepath.Join(out, "codex"))
	sessions, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != len(wantCodexUsage) {
		t.Fatalf("listed %d sample Codex sessions, want %d", len(sessions), len(wantCodexUsage))
	}

	clock := nowFunc()
	for _, s := range sessions {
		want, ok := wantCodexUsage[s.ID]
		if !ok {
			t.Errorf("unexpected session %s", s.ID)
			continue
		}
		got := s.Usage
		if got.CostUSD <= 0 || got.UnknownModel {
			t.Errorf("session %s: cost %v unknownModel %v, want a priced model", s.ID, got.CostUSD, got.UnknownModel)
		}
		got.CostUSD, got.UnknownModel, got.Breakdown = 0, false, nil
		if !reflect.DeepEqual(got, want) {
			t.Errorf("session %s: usage %+v, want %+v", s.ID, got, want)
		}
		if s.Usage.Total() != wantCodexTotal[s.ID] {
			t.Errorf("session %s: Total() = %d, want %d", s.ID, s.Usage.Total(), wantCodexTotal[s.ID])
		}
		if s.FirstPrompt == "" || s.CWD == "" || s.GitBranch == "" || s.AgentVersion == "" || len(s.Models) == 0 {
			t.Errorf("session %s: missing head metadata %+v", s.ID, s)
		}
		if s.FirstPrompt != "" && strings.HasPrefix(s.FirstPrompt, "<") {
			t.Errorf("session %s: first prompt is a harness message: %q", s.ID, s.FirstPrompt)
		}

		m := rolloutKey.FindStringSubmatch(s.Key)
		if m == nil {
			t.Errorf("session %s: key %q is not YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl", s.ID, s.Key)
		} else {
			start := s.StartedAt.UTC()
			if dir := m[1] + "-" + m[2] + "-" + m[3]; dir != start.Format("2006-01-02") {
				t.Errorf("session %s: date dir %s, started %v", s.ID, dir, start)
			}
			if m[4] != start.Format("2006-01-02T15-04-05") || m[5] != s.ID {
				t.Errorf("session %s: file name %q does not match start %v / id", s.ID, s.Key, start)
			}
		}
		if age := clock.Sub(s.StartedAt); age < 0 || age > 14*24*time.Hour {
			t.Errorf("session %s: started %v, want within 14 days before %v", s.ID, s.StartedAt, clock)
		}

		d, err := src.Detail(context.Background(), s.Key)
		if err != nil {
			t.Fatalf("detail %s: %v", s.Key, err)
		}
		paired := 0
		for _, turn := range d.Turns {
			for _, b := range turn.Blocks {
				if b.Kind == agentsessions.BlockToolCall && b.Paired && b.Output != "" {
					paired++
				}
			}
		}
		if paired == 0 {
			t.Errorf("session %s: no paired tool call in detail", s.ID)
		}
	}
}

// TestRun_CodexSessionsReplaceStaleDays: a forced re-run on a later day
// rewrites the rollouts under the new date and leaves no stale copies, so
// the reader still lists exactly the sample sessions.
func TestRun_CodexSessionsReplaceStaleDays(t *testing.T) {
	pinClock(t)
	out := filepath.Join(t.TempDir(), "demo")
	if err := run([]string{"--out", out}, io.Discard); err != nil {
		t.Fatalf("first run: %v", err)
	}
	nowFunc = func() time.Time { return time.Date(2026, 3, 20, 9, 30, 0, 0, time.UTC) }
	if err := run([]string{"--out", out, "--force"}, io.Discard); err != nil {
		t.Fatalf("forced run: %v", err)
	}
	sessions, err := codex.New("codex", filepath.Join(out, "codex")).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != len(wantCodexUsage) {
		t.Fatalf("listed %d sessions after a later re-run, want %d", len(sessions), len(wantCodexUsage))
	}
	for _, s := range sessions {
		if s.StartedAt.Before(time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("session %s (%s) is a stale copy from the first run", s.ID, s.Key)
		}
	}
}
