package ui_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// fakeSource is an in-memory agentsessions.Source.
type fakeSource struct {
	id, agent string
	sessions  []agentsessions.Session
	err       error
}

func (f fakeSource) ID() string                                            { return f.id }
func (f fakeSource) Agent() string                                         { return f.agent }
func (f fakeSource) Fingerprint(context.Context) (string, error)           { return "fp", nil }
func (f fakeSource) List(context.Context) ([]agentsessions.Session, error) { return f.sessions, f.err }
func (f fakeSource) Detail(context.Context, string) (*agentsessions.Detail, error) {
	return nil, agentsessions.ErrNotFound
}

func day(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// agentSources: one Claude session (alpha, opus, $2) and one Codex session
// (proj, gpt-5, $1).
func agentSources() []agentsessions.Source {
	return []agentsessions.Source{
		fakeSource{id: "claude-code/default", agent: "claude-code", sessions: []agentsessions.Session{{
			Key: "-tmp-alpha/s1", ID: "s1", Project: "alpha", ProjectKey: "/tmp/alpha", GitBranch: "main",
			Models: []string{"claude-opus-4-8"}, FirstPrompt: "Fix the claude bug",
			StartedAt: day("2026-01-02T10:00:00Z"), EndedAt: day("2026-01-02T11:00:00Z"),
			Usage: agentsessions.Usage{Input: 100, Output: 50, CostUSD: 2},
		}}},
		fakeSource{id: "codex", agent: "codex", sessions: []agentsessions.Session{{
			Key: "2025/10/01/rollout-a.jsonl", ID: "c1", Project: "proj", ProjectKey: "/work/proj",
			Models: []string{"gpt-5"}, FirstPrompt: "hello codex",
			StartedAt: day("2025-10-01T10:00:00Z"), EndedAt: day("2025-10-01T10:30:00Z"),
			Usage: agentsessions.Usage{Input: 600, CacheRead: 400, Output: 200, Reasoning: 50, CostUSD: 1},
		}}},
	}
}

func agentMux(t *testing.T, sources []agentsessions.Source) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{AgentSessions: sources})
	return mux
}

func get(mux http.Handler, route string, htmx bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, route, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestAgentSessionsList_MergesAgentsWithAgentColumn covers S1: the list
// merges every source, shows the agent of each row and links each session to
// its source-scoped detail URL.
func TestAgentSessionsList_MergesAgentsWithAgentColumn(t *testing.T) {
	w := get(agentMux(t, agentSources()), "/agent-sessions/list", false)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(strings.ToLower(body)), "<!doctype html>") {
		t.Fatalf("status %d, want a 200 full page (body %.200s)", w.Code, body)
	}
	for _, want := range []string{
		"Fix the claude bug", "hello codex", ">Claude Code<", ">Codex<",
		`href="/agent-sessions/claude-code/default/-tmp-alpha/s1"`,
		`href="/agent-sessions/codex/2025/10/01/rollout-a.jsonl"`,
		`name="agent"`, `name="source"`, `name="project"`, `name="model"`, `name="q"`, `name="from"`, `name="to"`, `name="sort"`, `name="order"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list lacks %q", want)
		}
	}
	// Default order: last activity, newest first → Claude (2026) before Codex.
	if strings.Index(body, "Fix the claude bug") > strings.Index(body, "hello codex") {
		t.Error("rows are not sorted by last activity, newest first")
	}
	if hx := get(agentMux(t, agentSources()), "/agent-sessions/list", true); strings.Contains(strings.ToLower(hx.Body.String()), "<!doctype") || !strings.Contains(hx.Body.String(), "hello codex") {
		t.Errorf("htmx list: want the bare list content, got %.200s", hx.Body.String())
	}
}

// TestAgentSessionsList_Filters covers the agent, source, project, model, q,
// from/to and sort/order filters.
func TestAgentSessionsList_Filters(t *testing.T) {
	mux := agentMux(t, agentSources())
	for _, tc := range []struct {
		query     string
		want, not string
	}{
		{"agent=codex", "hello codex", "Fix the claude bug"},
		{"agent=claude-code", "Fix the claude bug", "hello codex"},
		{"source=codex", "hello codex", "Fix the claude bug"},
		{"project=alpha", "Fix the claude bug", "hello codex"},
		{"model=gpt-5", "hello codex", "Fix the claude bug"},
		{"q=CLAUDE+BUG", "Fix the claude bug", "hello codex"},
		{"from=2026-01-01", "Fix the claude bug", "hello codex"},
		{"to=2026-01-01", "hello codex", "Fix the claude bug"},
	} {
		body := get(mux, "/agent-sessions/list?"+tc.query, false).Body.String()
		if !strings.Contains(body, tc.want) || strings.Contains(body, tc.not) {
			t.Errorf("?%s: want %q and not %q", tc.query, tc.want, tc.not)
		}
	}
	body := get(mux, "/agent-sessions/list?sort=cost&order=asc", false).Body.String()
	if strings.Index(body, "hello codex") > strings.Index(body, "Fix the claude bug") {
		t.Error("sort=cost&order=asc: the cheaper Codex session should come first")
	}
	if !strings.Contains(get(mux, "/agent-sessions/list?agent=nobody", false).Body.String(), "No sessions match these filters.") {
		t.Error("an empty result does not render the empty state")
	}
}

// TestAgentSessionsList_PaginatesAndReportsFailingSource: pages of 50 with
// prev/next links that keep the filters, and a per-source notice when one
// source fails while the others still list.
func TestAgentSessionsList_PaginatesAndReportsFailingSource(t *testing.T) {
	var many []agentsessions.Session
	for i := 0; i < 55; i++ {
		many = append(many, agentsessions.Session{
			Key: fmt.Sprintf("k%02d.jsonl", i), ID: fmt.Sprintf("id%02d", i), Project: "proj",
			FirstPrompt: fmt.Sprintf("prompt-%02d", i), EndedAt: day("2025-10-01T10:00:00Z").Add(time.Duration(i) * time.Minute),
		})
	}
	sources := []agentsessions.Source{
		fakeSource{id: "codex", agent: "codex", sessions: many},
		fakeSource{id: "claude-code/work", agent: "claude-code", err: errors.New("permission denied")},
	}
	mux := agentMux(t, sources)

	p1 := get(mux, "/agent-sessions/list?agent=codex", false).Body.String()
	if !strings.Contains(p1, "prompt-54") || strings.Contains(p1, "prompt-04") || !strings.Contains(p1, "Page 1 of 2") {
		t.Errorf("page 1: want the 50 newest rows and 'Page 1 of 2'")
	}
	if !strings.Contains(p1, `href="/agent-sessions/list?agent=codex&amp;page=2"`) {
		t.Errorf("page 1 lacks the next link keeping the filters:\n%s", p1)
	}
	p2 := get(mux, "/agent-sessions/list?agent=codex&page=2", false).Body.String()
	if !strings.Contains(p2, "prompt-04") || strings.Contains(p2, "prompt-54") || !strings.Contains(p2, "Page 2 of 2") {
		t.Error("page 2: want the 5 oldest rows")
	}
	if !strings.Contains(p1, "Could not read claude-code/work (Claude Code); its sessions are not shown.") {
		t.Error("the failing source is not reported")
	}
	if strings.Contains(p1, "permission denied") {
		t.Error("the source error text leaked into the page")
	}
}

// TestAgentSessionsOverview_RendersChartsIsland: the overview page carries
// the usage-charts island (which fetches /api/agent-sessions/stats) and the
// tabs to the list.
func TestAgentSessionsOverview_RendersChartsIsland(t *testing.T) {
	w := get(agentMux(t, agentSources()), "/agent-sessions", false)
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	for _, want := range []string{`data-island="agent-usage-charts"`, `href="/agent-sessions/list"`, "Agent sessions", "costByAgent"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview lacks %q", want)
		}
	}
}

// TestAgentSessions_GatedOnLiveSources: without a live agent-session source
// the pages answer 503; the legacy /cc-* URLs always redirect (301) to the
// neutral pages keeping their query string.
func TestAgentSessions_GatedOnLiveSources(t *testing.T) {
	var live ui.LiveHandles
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Live: func(*http.Request) ui.LiveHandles { return live }})
	for _, route := range []string{"/agent-sessions", "/agent-sessions/list"} {
		if w := get(mux, route, false); w.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s without sources: status %d, want 503", route, w.Code)
		}
	}
	for _, tc := range []struct{ from, to string }{
		{"/cc-overview", "/agent-sessions"},
		{"/cc-overview?tab=sessions", "/agent-sessions/list?agent=claude-code"},
		{"/cc-sessions", "/agent-sessions/list?agent=claude-code"},
		{"/cc-sessions?project=alpha", "/agent-sessions/list?agent=claude-code&project=alpha"},
		{"/cc-sessions/list?project=alpha&account=work&cursor=50&append=true", "/agent-sessions/list?agent=claude-code&project=alpha&source=claude-code%2Fwork"},
	} {
		w := get(mux, tc.from, false)
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != tc.to {
			t.Errorf("GET %s: %d → %q, want 301 → %q", tc.from, w.Code, w.Header().Get("Location"), tc.to)
		}
	}
	live.AgentSessions = agentSources()
	if w := get(mux, "/agent-sessions/list", false); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello codex") {
		t.Errorf("GET /agent-sessions/list with sources: status %d", w.Code)
	}
}
