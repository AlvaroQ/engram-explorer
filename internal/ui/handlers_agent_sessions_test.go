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

// manyCodex returns n Codex sessions; prompt-NN ends NN minutes after the
// first, so the default order (newest first) lists prompt-(n-1) first.
func manyCodex(n int) []agentsessions.Session {
	var many []agentsessions.Session
	for i := 0; i < n; i++ {
		at := day("2025-10-01T10:00:00Z").Add(time.Duration(i) * time.Minute)
		many = append(many, agentsessions.Session{
			Key: fmt.Sprintf("k%02d.jsonl", i), ID: fmt.Sprintf("id%02d", i), Project: "proj",
			FirstPrompt: fmt.Sprintf("prompt-%02d", i), StartedAt: at, EndedAt: at,
			Usage: agentsessions.Usage{CostUSD: float64(i)},
		})
	}
	return many
}

// TestAgentSessionsList_LoadMoreAndFailingSource (S10): the first page shows
// 50 rows and a load-more sentinel row whose button fetches the next page
// (append=true, same filters) into its own row, with a no-JS href to the
// numbered page; a failing source is reported while the others still list.
func TestAgentSessionsList_LoadMoreAndFailingSource(t *testing.T) {
	sources := []agentsessions.Source{
		fakeSource{id: "codex", agent: "codex", sessions: manyCodex(55)},
		fakeSource{id: "claude-code/work", agent: "claude-code", err: errors.New("permission denied")},
	}
	mux := agentMux(t, sources)

	p1 := get(mux, "/agent-sessions/list?agent=codex", false).Body.String()
	if !strings.Contains(p1, "prompt-54") || strings.Contains(p1, "prompt-04") {
		t.Errorf("page 1: want the 50 newest rows")
	}
	for _, want := range []string{
		`<tr id="as-load-more-row">`,
		`hx-get="/agent-sessions/list?agent=codex&amp;append=true&amp;page=2"`,
		`href="/agent-sessions/list?agent=codex&amp;page=2"`,
		`hx-target="closest tr"`, `hx-swap="outerHTML"`,
		">Load more<", "50 of 55",
	} {
		if !strings.Contains(p1, want) {
			t.Errorf("page 1 lacks %q", want)
		}
	}
	if strings.Contains(p1, "Page 1 of 2") {
		t.Error("page 1 still renders the numbered pager")
	}
	if !strings.Contains(p1, "Could not read claude-code/work (Claude Code); its sessions are not shown.") {
		t.Error("the failing source is not reported")
	}
	if strings.Contains(p1, "permission denied") {
		t.Error("the source error text leaked into the page")
	}
}

// TestAgentSessionsList_AppendReturnsRowsOnly: append=true answers only the
// next rows (no page chrome, table or filter form, HTMX or not); the last page
// carries no sentinel.
func TestAgentSessionsList_AppendReturnsRowsOnly(t *testing.T) {
	mux := agentMux(t, []agentsessions.Source{fakeSource{id: "codex", agent: "codex", sessions: manyCodex(55)}})
	for _, htmx := range []bool{true, false} {
		w := get(mux, "/agent-sessions/list?agent=codex&append=true&page=2", htmx)
		body := w.Body.String()
		if w.Code != http.StatusOK {
			t.Fatalf("htmx=%v: status %d", htmx, w.Code)
		}
		if !strings.Contains(body, "prompt-04") || !strings.Contains(body, "prompt-00") || strings.Contains(body, "prompt-05") {
			t.Errorf("htmx=%v: want exactly the 5 oldest rows, got %.300s", htmx, body)
		}
		for _, not := range []string{"<html", "<table", "<tbody", `name="agent"`, "as-load-more-row", "agent-pager"} {
			if strings.Contains(body, not) {
				t.Errorf("htmx=%v: append response contains %q", htmx, not)
			}
		}
		if !strings.HasPrefix(strings.TrimSpace(body), "<tr") {
			t.Errorf("htmx=%v: append response must start with a row, got %.80s", htmx, body)
		}
	}
	// A middle page keeps a sentinel pointing at the following page.
	mid := get(manyMux(t, 120), "/agent-sessions/list?append=true&page=2", true).Body.String()
	if !strings.Contains(mid, `hx-get="/agent-sessions/list?append=true&amp;page=3"`) || !strings.Contains(mid, "100 of 120") {
		t.Errorf("page 2 of 3: want a sentinel to page 3 reporting 100 of 120, got %.400s", mid)
	}
}

func manyMux(t *testing.T, n int) *http.ServeMux {
	return agentMux(t, []agentsessions.Source{fakeSource{id: "codex", agent: "codex", sessions: manyCodex(n)}})
}

// TestAgentSessionsList_LoadMoreKeepsFilters: the next-page URL carries every
// active filter and the sort, so appended rows continue the same listing.
func TestAgentSessionsList_LoadMoreKeepsFilters(t *testing.T) {
	q := "agent=codex&source=codex&project=proj&model=&q=prompt&from=2025-01-01&to=2025-12-31&sort=cost&order=asc"
	body := get(manyMux(t, 55), "/agent-sessions/list?"+q, false).Body.String()
	want := `hx-get="/agent-sessions/list?agent=codex&amp;append=true&amp;from=2025-01-01&amp;order=asc&amp;page=2&amp;project=proj&amp;q=prompt&amp;sort=cost&amp;source=codex&amp;to=2025-12-31"`
	if !strings.Contains(body, want) {
		t.Errorf("load-more URL does not keep the filters; want %s", want)
	}
	// sort=cost asc: the cheapest (prompt-00) first; page 2 continues at prompt-50.
	if !strings.Contains(get(manyMux(t, 55), "/agent-sessions/list?"+q+"&append=true&page=2", true).Body.String(), "prompt-50") {
		t.Error("page 2 does not continue the sorted listing")
	}
}

// TestAgentSessionsList_NumberedPageBookmark: ?page=N still renders that page
// as a full page, with a link back to the previous one (no-JS fallback).
func TestAgentSessionsList_NumberedPageBookmark(t *testing.T) {
	p2 := get(manyMux(t, 55), "/agent-sessions/list?agent=codex&page=2", false).Body.String()
	if !strings.Contains(p2, "prompt-04") || strings.Contains(p2, "prompt-54") {
		t.Error("page 2: want the 5 oldest rows")
	}
	if !strings.Contains(p2, `href="/agent-sessions/list?agent=codex"`) || !strings.Contains(p2, ">Previous<") {
		t.Error("page 2 lacks the previous-page link")
	}
	if strings.Contains(p2, "as-load-more-row") {
		t.Error("the last page must not render a load-more sentinel")
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

// TestAgentSessionsList_MarkersAreAgentSpecific (S10): the Claude Code marker
// set strips Claude prompts only; a Codex prompt with the same text is shown
// verbatim, without the attachment clip.
func TestAgentSessionsList_MarkersAreAgentSpecific(t *testing.T) {
	prompt := "<system-reminder>be brief</system-reminder> shared prompt"
	mk := func(id, agent string) agentsessions.Source {
		return fakeSource{id: id, agent: agent, sessions: []agentsessions.Session{{Key: id + ".jsonl", ID: id, FirstPrompt: prompt}}}
	}
	mux := agentMux(t, []agentsessions.Source{mk("claude-code/default", "claude-code"), mk("codex", "codex")})

	claude := get(mux, "/agent-sessions/list?agent=claude-code", false).Body.String()
	if strings.Contains(claude, "&lt;system-reminder&gt;") || !strings.Contains(claude, "cc-prompt-clip") {
		t.Error("claude-code row: the system-reminder marker should be stripped behind the clip")
	}
	codex := get(mux, "/agent-sessions/list?agent=codex", false).Body.String()
	if !strings.Contains(codex, "&lt;system-reminder&gt;be brief&lt;/system-reminder&gt; shared prompt") || strings.Contains(codex, "cc-prompt-clip") {
		t.Error("codex row: the prompt must not be stripped with Claude marker tags")
	}
}

// TestAgentSessionsOverview_AgentTabs (S2): the overview shows one tab per
// live agent plus "All agents", each linking to ?agent=<kind>; the selected
// agent is aria-current, is handed to the charts island and carried by the
// Sessions tab.
func TestAgentSessionsOverview_AgentTabs(t *testing.T) {
	mux := agentMux(t, agentSources())
	all := get(mux, "/agent-sessions", false).Body.String()
	for _, want := range []string{
		`<a href="/agent-sessions" class="btn btn-primary btn-sm" aria-current="page">All agents</a>`,
		`<a href="/agent-sessions?agent=claude-code" class="btn btn-secondary btn-sm">Claude Code</a>`,
		`<a href="/agent-sessions?agent=codex" class="btn btn-secondary btn-sm">Codex</a>`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("overview lacks %s", want)
		}
	}
	if strings.Contains(all, "&#34;agent&#34;:") {
		t.Error("unfiltered overview must not hand an agent to the island")
	}

	codex := get(mux, "/agent-sessions?agent=codex", true).Body.String()
	for _, want := range []string{
		`<a href="/agent-sessions" class="btn btn-secondary btn-sm">All agents</a>`,
		`<a href="/agent-sessions?agent=codex" class="btn btn-primary btn-sm" aria-current="page">Codex</a>`,
		`href="/agent-sessions/list?agent=codex"`,
		"&#34;agent&#34;:&#34;codex&#34;",
	} {
		if !strings.Contains(codex, want) {
			t.Errorf("?agent=codex overview lacks %s", want)
		}
	}

	// Only the live agents get a tab.
	only := get(agentMux(t, agentSources()[1:]), "/agent-sessions", false).Body.String()
	if strings.Contains(only, "?agent=claude-code") || !strings.Contains(only, "?agent=codex") {
		t.Error("tabs must list only the agents of the live sources")
	}

	// Unknown agent: same convention as the list and the stats API — 200,
	// the filter passes through (empty result), no tab is current.
	w := get(mux, "/agent-sessions?agent=nobody", false)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `class="btn btn-primary btn-sm" aria-current="page"`) || !strings.Contains(w.Body.String(), "&#34;agent&#34;:&#34;nobody&#34;") {
		t.Errorf("?agent=nobody: status %d; want 200, no current tab and the filter handed to the island", w.Code)
	}
}

// TestAgentSessionsList_AgentFilterKeepsOverviewTab: the list filtered by
// agent links its Overview tab to the same agent.
func TestAgentSessionsList_AgentFilterKeepsOverviewTab(t *testing.T) {
	body := get(agentMux(t, agentSources()), "/agent-sessions/list?agent=codex", false).Body.String()
	if !strings.Contains(body, `href="/agent-sessions?agent=codex"`) || strings.Contains(body, "Fix the claude bug") {
		t.Error("?agent=codex list: want the Overview tab scoped to codex and only Codex rows")
	}
}
