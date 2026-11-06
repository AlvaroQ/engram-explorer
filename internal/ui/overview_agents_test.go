package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

type kpiSource struct {
	id, agent string
	sessions  []agentsessions.Session
	err       error
}

func (s kpiSource) ID() string                                  { return s.id }
func (s kpiSource) Agent() string                               { return s.agent }
func (s kpiSource) Fingerprint(context.Context) (string, error) { return "fp", nil }
func (s kpiSource) List(context.Context) ([]agentsessions.Session, error) {
	return s.sessions, s.err
}
func (s kpiSource) Detail(context.Context, string) (*agentsessions.Detail, error) {
	return nil, agentsessions.ErrNotFound
}

func kpiSession(started time.Time, cost float64, unknown bool) agentsessions.Session {
	return agentsessions.Session{StartedAt: started, Usage: agentsessions.Usage{CostUSD: cost, UnknownModel: unknown}}
}

// TestOverviewAgentKPIs: AI conversations count the sessions started in the
// last 7 UTC days (the memories window) per live agent; the spend sums
// CostUSD of the sessions started in the current UTC calendar month, so a
// session on the last day of the previous month never counts; an unpriced
// model marks the total and its agent as a lower bound.
func TestOverviewAgentKPIs(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)
	claude := kpiSource{id: "claude-code", agent: "claude-code", sessions: []agentsessions.Session{
		kpiSession(time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), 2.50, false),
		kpiSession(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 1.25, false),    // first instant of the month
		kpiSession(time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC), 9.00, false), // previous month, within 7 days
		kpiSession(time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), 4.00, false),   // first day of the 7-day window
		kpiSession(time.Date(2026, 10, 26, 23, 59, 0, 0, time.UTC), 4.00, false), // just before the window
		kpiSession(time.Time{}, 3.00, false),                                     // unknown start: never counted
	}}
	codex := kpiSource{id: "codex", agent: "codex", sessions: []agentsessions.Session{
		kpiSession(time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC), 0.75, true),
	}}

	got, err := buildOverviewAgentKPIs(context.Background(), []agentsessions.Source{claude, codex}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Conversations != 5 {
		t.Errorf("Conversations = %d, want 5 (claude 4 in the window + codex 1)", got.Conversations)
	}
	wantConv := []agentShare{{Agent: "claude-code", Sessions: 4}, {Agent: "codex", Sessions: 1}}
	if len(got.ConversationsByAgent) != 2 || got.ConversationsByAgent[0].Agent != wantConv[0].Agent ||
		got.ConversationsByAgent[0].Sessions != 4 || got.ConversationsByAgent[1].Sessions != 1 {
		t.Errorf("ConversationsByAgent = %+v, want %+v", got.ConversationsByAgent, wantConv)
	}
	if got.Spend.CostUSD != 4.50 || !got.Spend.UnknownModel {
		t.Errorf("Spend = %+v, want $4.50 (2.50+1.25+0.75) flagged unknown", got.Spend)
	}
	if len(got.SpendByAgent) != 2 || got.SpendByAgent[0].Usage.CostUSD != 3.75 || got.SpendByAgent[0].Usage.UnknownModel ||
		got.SpendByAgent[1].Usage.CostUSD != 0.75 || !got.SpendByAgent[1].Usage.UnknownModel {
		t.Errorf("SpendByAgent = %+v, want claude $3.75 (priced), codex $0.75 (unknown)", got.SpendByAgent)
	}

	// An agent with no session in the window still shows, with zero.
	quiet := kpiSource{id: "codex", agent: "codex"}
	got, _ = buildOverviewAgentKPIs(context.Background(), []agentsessions.Source{claude, quiet}, now)
	if len(got.ConversationsByAgent) != 2 || got.ConversationsByAgent[1].Agent != "codex" || got.ConversationsByAgent[1].Sessions != 0 {
		t.Errorf("quiet agent: ConversationsByAgent = %+v, want codex with 0", got.ConversationsByAgent)
	}

	// A failing source is reported, the others still count.
	broken := kpiSource{id: "codex", agent: "codex", err: errors.New("unreadable")}
	got, err = buildOverviewAgentKPIs(context.Background(), []agentsessions.Source{claude, broken}, now)
	if err != nil || got.Conversations != 4 || got.SourceErrors != 1 {
		t.Errorf("broken source: %+v, %v; want 4 conversations and 1 source error", got, err)
	}
}

// TestOverviewAgentKPIs_MonthBoundary: half an hour into a new UTC month the
// spend only counts the session started at its first instant, while the
// 7-day conversations window still counts the one from the last minute of
// the previous month.
func TestOverviewAgentKPIs_MonthBoundary(t *testing.T) {
	now := time.Date(2026, 12, 1, 0, 30, 0, 0, time.UTC)
	src := kpiSource{id: "claude-code", agent: "claude-code", sessions: []agentsessions.Session{
		kpiSession(time.Date(2026, 11, 30, 23, 59, 0, 0, time.UTC), 2.00, false),
		kpiSession(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 0.50, false),
	}}
	got, err := buildOverviewAgentKPIs(context.Background(), []agentsessions.Source{src}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spend.CostUSD != 0.50 {
		t.Errorf("Spend = $%.2f, want $0.50 (only the session of December)", got.Spend.CostUSD)
	}
	if len(got.SpendByAgent) != 1 || got.SpendByAgent[0].Sessions != 1 {
		t.Errorf("SpendByAgent = %+v, want one December session", got.SpendByAgent)
	}
	if got.Conversations != 2 {
		t.Errorf("Conversations = %d, want 2 (both within the last 7 days)", got.Conversations)
	}
}
