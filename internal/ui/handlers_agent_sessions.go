package ui

import (
	"net/http"
	"net/url"
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
// transcript scan never blocks the page.
func handleAgentSessionsOverview(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		if IsHTMX(r) {
			renderDeps(w, r, d, AgentSessionsOverviewPartial(lang))
			return
		}
		renderDeps(w, r, d, AgentSessionsOverviewPage(lang, themeForRequest(r), sidebarStateForRequest(r)))
	}
}

// handleAgentSessionsList serves GET /agent-sessions/list: every live source
// merged, filtered, sorted and paginated (agentsessions.ListSessions).
func handleAgentSessionsList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		q := agentListQueryFrom(r)
		res, err := agentsessions.ListSessions(r.Context(), d.AgentSessions, q.params())
		if err != nil {
			RenderError(w, r, "Failed to load agent sessions: ", err)
			return
		}
		v := agentListView{Result: res, Query: q, Lang: lang}
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

// pageURL is the list URL of page n with the same filters.
func (q agentListQuery) pageURL(n int) string {
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
// Load-more paging (cursor/append) has no neutral equivalent and is dropped.
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
