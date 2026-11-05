package ui

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// ---------------------------------------------------------------------------
// Host pages: overview and list (mounted once, gated on a live source)
// ---------------------------------------------------------------------------

// handleAgentSessionsOverview serves GET /agent-sessions: the usage charts
// island, which fetches /api/agent-sessions/stats on its own so the full
// transcript scan never blocks the page. ?agent=<kind> scopes the page to one
// agent (tab row: "All agents" plus one tab per live agent). Like the list and
// the stats API, an unknown agent is not an error: the filter passes through
// and matches nothing.
func handleAgentSessionsOverview(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := agentOverviewView{
			Lang:   langForRequest(r),
			Agent:  r.URL.Query().Get("agent"),
			Agents: liveAgents(d.AgentSessions),
		}
		if IsHTMX(r) {
			renderDeps(w, r, d, AgentSessionsOverviewPartial(v))
			return
		}
		renderDeps(w, r, d, AgentSessionsOverviewPage(v, themeForRequest(r), sidebarStateForRequest(r)))
	}
}

// agentOverviewView is the data of the overview page.
type agentOverviewView struct {
	Lang   string
	Agent  string   // selected agent kind ("" = all agents)
	Agents []string // agent kinds of the live sources, first-seen order
}

// liveAgents returns the distinct agent kinds of sources, in source order.
func liveAgents(sources []agentsessions.Source) []string {
	var out []string
	for _, src := range sources {
		if a := src.Agent(); a != "" && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

// agentScopedURL is path with ?agent=<kind> when an agent is selected.
func agentScopedURL(path, agent string) string {
	if agent == "" {
		return path
	}
	return path + "?" + url.Values{"agent": {agent}}.Encode()
}

// handleAgentSessionsList serves GET /agent-sessions/list: every live source
// merged, filtered, sorted and paginated (agentsessions.ListSessions).
// append=true is the load-more request: it answers only the rows of the page
// plus the next load-more sentinel, which replace the clicked sentinel row.
func handleAgentSessionsList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := agentListQueryFrom(r)
		res, err := agentsessions.ListSessions(r.Context(), d.AgentSessions, q.params())
		if err != nil {
			RenderError(w, r, "errors.loadAgentSessions", err)
			return
		}
		v := agentListView{Result: res, Query: q, Lang: lang}
		if r.URL.Query().Get("append") == "true" {
			renderDeps(w, r, d, AgentSessionsListRows(v))
			return
		}
		if IsHTMX(r) {
			renderDeps(w, r, d, AgentSessionsListPartial(v))
			return
		}
		renderDeps(w, r, d, AgentSessionsListPage(v, themeForRequest(r), sidebarStateForRequest(r)))
	}
}

// agentListQuery is the list filter state as submitted (form values).
type agentListQuery struct {
	Agent, Source, Project, Model, Q, From, To, Sort, Order string
	Page                                                    int
}

func agentListQueryFrom(r *http.Request) agentListQuery {
	v := r.URL.Query()
	page, _ := strconv.Atoi(v.Get("page"))
	return agentListQuery{
		Agent: v.Get("agent"), Source: v.Get("source"), Project: v.Get("project"), Model: v.Get("model"),
		Q: strings.TrimSpace(v.Get("q")), From: v.Get("from"), To: v.Get("to"),
		Sort: v.Get("sort"), Order: v.Get("order"), Page: max(page, 1),
	}
}

// params converts the form state; From/To are calendar days (UTC), To
// inclusive. Unparseable dates are ignored.
func (q agentListQuery) params() agentsessions.ListParams {
	p := agentsessions.ListParams{
		Query: q.Q, Agent: q.Agent, Source: q.Source, Project: q.Project, Model: q.Model,
		Sort: q.Sort, Order: q.Order, Page: q.Page,
	}
	if t, err := time.Parse(time.DateOnly, q.From); err == nil {
		p.From = t
	}
	if t, err := time.Parse(time.DateOnly, q.To); err == nil {
		p.To = t.AddDate(0, 0, 1)
	}
	return p
}

// pageURL is the list URL of page n with the same filters (the no-JS
// fallback and bookmarkable form of a page).
func (q agentListQuery) pageURL(n int) string {
	return listURL(q.pageValues(n))
}

// appendURL is the load-more URL of page n: the same filters and sort plus
// append=true, so the handler answers only the rows.
func (q agentListQuery) appendURL(n int) string {
	v := q.pageValues(n)
	v.Set("append", "true")
	return listURL(v)
}

func (q agentListQuery) pageValues(n int) url.Values {
	v := url.Values{}
	for k, val := range map[string]string{
		"agent": q.Agent, "source": q.Source, "project": q.Project, "model": q.Model,
		"q": q.Q, "from": q.From, "to": q.To, "sort": q.Sort, "order": q.Order,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if n > 1 {
		v.Set("page", strconv.Itoa(n))
	}
	return v
}

func listURL(v url.Values) string {
	if len(v) == 0 {
		return agentsessions.RoutePrefix + "/list"
	}
	return agentsessions.RoutePrefix + "/list?" + v.Encode()
}

// agentListView is the data of the list page.
type agentListView struct {
	Result agentsessions.ListResult
	Query  agentListQuery
	Lang   string
}

// shown is the number of rows listed up to and including the current page.
func (v agentListView) shown() int {
	return (v.Result.Page-1)*v.Result.PageSize + len(v.Result.Items)
}

// ---------------------------------------------------------------------------
// Legacy /cc-* URLs (compat): permanent redirects to the neutral pages
// ---------------------------------------------------------------------------

// claudeAgent is the agent kind the legacy Claude Code URLs are scoped to.
const claudeAgent = "claude-code"

// redirectCCOverview serves GET /cc-overview: the overview tab moves to
// /agent-sessions, the sessions tab (?tab=sessions) to the Claude list.
func redirectCCOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("tab") == "sessions" {
		redirectCCList(w, r)
		return
	}
	http.Redirect(w, r, agentsessions.RoutePrefix, http.StatusMovedPermanently)
}

// redirectCCList serves GET /cc-sessions and /cc-sessions/list: the Claude
// list with the same project filter; ?account=<id> becomes the source filter.
// The legacy cursor paging (cursor/append) is dropped: the neutral list
// pages by number (its own load-more uses ?page=N&append=true).
func redirectCCList(w http.ResponseWriter, r *http.Request) {
	old := r.URL.Query()
	v := url.Values{"agent": {claudeAgent}}
	if p := old.Get("project"); p != "" {
		v.Set("project", p)
	}
	if a := old.Get("account"); a != "" {
		v.Set("source", claudeAgent+"/"+a)
	}
	http.Redirect(w, r, agentsessions.RoutePrefix+"/list?"+v.Encode(), http.StatusMovedPermanently)
}
