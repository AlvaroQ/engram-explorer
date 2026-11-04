package agentsessions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

type groupRow struct {
	Key      string
	Sessions int
	Total    int64
}

func rows(gs []agentsessions.GroupStat) []groupRow {
	out := make([]groupRow, 0, len(gs))
	for _, g := range gs {
		out = append(out, groupRow{g.Key, g.Sessions, g.Usage.Total()})
	}
	return out
}

func TestStatsAggregatesPerAgentProjectModelDay(t *testing.T) {
	res, err := agentsessions.Stats(context.Background(), fixtureSources(), agentsessions.ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 5 {
		t.Fatalf("Sessions = %d", res.Sessions)
	}
	if res.Totals.Total() != 577 || res.Totals.CostUSD != 6.5 {
		t.Fatalf("Totals = %+v (total %d)", res.Totals, res.Totals.Total())
	}
	if !res.HasUnknownModel {
		t.Fatal("HasUnknownModel must reflect the o3 session")
	}

	if got, want := rows(res.ByAgent), []groupRow{{"claude-code", 3, 170}, {"codex", 2, 407}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ByAgent = %v, want %v", got, want)
	}
	// Projects ordered by cost desc, then tokens desc, then key.
	if got, want := rows(res.ByProject), []groupRow{{"engram", 3, 520}, {"explorer", 1, 50}, {"other", 1, 7}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ByProject = %v, want %v", got, want)
	}
	// Without a per-model split: single model gets everything; multi-model goes to "unknown".
	if got, want := rows(res.ByModel), []groupRow{
		{"gpt-5-codex", 1, 400}, {"opus", 1, 110}, {"sonnet", 1, 50}, {"unknown", 1, 10}, {"o3", 1, 7},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ByModel = %v, want %v", got, want)
	}

	if len(res.OverTime) != 2 {
		t.Fatalf("OverTime = %+v", res.OverTime)
	}
	d0, d1 := res.OverTime[0], res.OverTime[1]
	if d0.Day != "2026-10-01" || d0.Sessions != 3 || d0.Usage.Total() != 520 {
		t.Fatalf("day0 = %+v", d0)
	}
	if d0.ByAgent["claude-code"].Total() != 120 || d0.ByAgent["codex"].Total() != 400 {
		t.Fatalf("day0 per agent = %+v", d0.ByAgent)
	}
	if d1.Day != "2026-10-02" || d1.Sessions != 2 || d1.ByAgent["claude-code"].Total() != 50 || d1.ByAgent["codex"].Total() != 7 {
		t.Fatalf("day1 = %+v", d1)
	}

	var top []string
	for _, s := range res.TopSessions {
		top = append(top, s.Source+"/"+s.Key)
	}
	if want := []string{"cc-main/c2", "codex/x1", "cc-main/c1", "cc-main/c3", "codex/x2"}; !reflect.DeepEqual(top, want) {
		t.Fatalf("TopSessions = %v, want %v", top, want)
	}
	if res.TopSessions[0].Agent != "claude-code" || res.TopSessions[0].Project != "explorer" || res.TopSessions[0].ID != "id-c2" {
		t.Fatalf("top session fields = %+v", res.TopSessions[0])
	}
	if res.GeneratedAt.IsZero() {
		t.Fatal("GeneratedAt must be set")
	}
}

func TestStatsUsesPerModelSplitAndFilters(t *testing.T) {
	s := sess("m1", "engram", 1, 2, []string{"opus", "haiku"}, agentsessions.Usage{Input: 30, CostUSD: 3})
	s.ModelUsage = map[string]agentsessions.Usage{"opus": {Input: 20, CostUSD: 2}, "haiku": {Input: 10, CostUSD: 1}}
	src := &fakeSource{id: "cc", agent: "claude-code", sessions: []agentsessions.Session{s,
		sess("m2", "other", 1, 2, []string{"opus"}, agentsessions.Usage{Input: 5}),
	}}
	res, err := agentsessions.Stats(context.Background(), []agentsessions.Source{src}, agentsessions.ListParams{Project: "engram"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 1 {
		t.Fatalf("filter ignored: %d sessions", res.Sessions)
	}
	if got, want := rows(res.ByModel), []groupRow{{"opus", 1, 20}, {"haiku", 1, 10}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ByModel = %v, want %v", got, want)
	}
}

func TestStatsTopSessionsCappedAndPromptTruncated(t *testing.T) {
	var ss []agentsessions.Session
	for i := 0; i < agentsessions.TopSessionsLimit+5; i++ {
		s := sess(string(rune('a'+i)), "p", 1, 2, nil, agentsessions.Usage{Input: int64(i)})
		s.FirstPrompt = "  line one\n\n" + strings.Repeat("é", 200)
		ss = append(ss, s)
	}
	res, err := agentsessions.Stats(context.Background(),
		[]agentsessions.Source{&fakeSource{id: "s", agent: "codex", sessions: ss}}, agentsessions.ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TopSessions) != agentsessions.TopSessionsLimit {
		t.Fatalf("TopSessions len = %d", len(res.TopSessions))
	}
	p := res.TopSessions[0].FirstPrompt
	if !strings.HasPrefix(p, "line one é") || !strings.HasSuffix(p, "…") || len([]rune(p)) != 81 {
		t.Fatalf("FirstPrompt label = %q (%d runes)", p, len([]rune(p)))
	}
}

func TestStatsIsolatesFailingSourcesAndEmpty(t *testing.T) {
	boom := errors.New("nope")
	res, err := agentsessions.Stats(context.Background(),
		[]agentsessions.Source{&fakeSource{id: "bad", agent: "codex", err: boom}}, agentsessions.ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 0 || len(res.Errors) != 1 || !errors.Is(res.Errors[0], boom) {
		t.Fatalf("res = %+v", res)
	}
	if res.ByAgent == nil || res.ByProject == nil || res.ByModel == nil || res.OverTime == nil || res.TopSessions == nil {
		t.Fatal("empty slices must be non-nil for JSON")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := agentsessions.Stats(ctx, fixtureSources(), agentsessions.ListParams{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
