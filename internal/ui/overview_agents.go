package ui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// agentShare is one agent's part of an overview agent KPI.
type agentShare struct {
	Agent    string
	Sessions int
	Usage    agentsessions.Usage
}

// overviewAgentKPIs feeds the "AI conversations" and "Agent spend this month"
// cards of the overview.
type overviewAgentKPIs struct {
	// Conversations counts the sessions started in the last 7 UTC days (the
	// same window as the memories card), split per live agent.
	Conversations        int
	ConversationsByAgent []agentShare
	// Spend sums Usage.CostUSD of the sessions started in the current UTC
	// calendar month; Spend.UnknownModel marks it as a lower bound.
	Spend        agentsessions.Usage
	SpendByAgent []agentShare
	// SourceErrors counts the sources that could not be read.
	SourceErrors int
}

// buildOverviewAgentKPIs reads every source once (agentsessions.Collect, whose
// sources cache their listing by fingerprint) and aggregates both windows.
// Sessions are attributed by StartedAt; one without a start time never
// counts. Every live agent gets a share, zero included, in source order.
func buildOverviewAgentKPIs(ctx context.Context, sources []agentsessions.Source, now time.Time) (overviewAgentKPIs, error) {
	c, err := agentsessions.Collect(ctx, sources)
	if err != nil {
		return overviewAgentKPIs{}, err
	}
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekFrom := today.AddDate(0, 0, -7)
	monthFrom := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	agents := liveAgents(sources)
	conv := make([]agentShare, len(agents))
	spend := make([]agentShare, len(agents))
	at := map[string]int{}
	for i, a := range agents {
		conv[i].Agent, spend[i].Agent = a, a
		at[a] = i
	}

	out := overviewAgentKPIs{SourceErrors: len(c.Errors)}
	for _, s := range c.Sessions {
		i, ok := at[s.Agent]
		if !ok || s.StartedAt.IsZero() {
			continue
		}
		started := s.StartedAt.UTC()
		if !started.Before(weekFrom) {
			out.Conversations++
			conv[i].Sessions++
		}
		if !started.Before(monthFrom) {
			out.Spend.Add(agentsessions.Usage{CostUSD: s.Usage.CostUSD, UnknownModel: s.Usage.UnknownModel})
			spend[i].Sessions++
			spend[i].Usage.Add(agentsessions.Usage{CostUSD: s.Usage.CostUSD, UnknownModel: s.Usage.UnknownModel})
		}
	}
	out.ConversationsByAgent, out.SpendByAgent = conv, spend
	return out, nil
}

// handleOverviewAgentsPartial serves GET /partials/overview-agents: the agent
// KPI cards of the overview, filled out of band (the page loads it lazily so
// the transcript scan never delays the first paint).
func handleOverviewAgentsPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k, err := buildOverviewAgentKPIs(r.Context(), d.AgentSessions, time.Now())
		if err != nil {
			RenderError(w, r, "", err)
			return
		}
		renderDeps(w, r, d, OverviewAgentKPIsFragment(k, langForRequest(r)))
	}
}

// overviewProjectHref drills into the most active project, or the project list.
func overviewProjectHref(project string) string {
	if project == "" {
		return "/projects"
	}
	return "/projects/" + url.PathEscape(project)
}

// ovFormatSpend is the spend value: a real $0.00 for no spend (not "—"),
// otherwise asFormatCost, whose "~" marks a lower bound (unpriced model).
func ovFormatSpend(u agentsessions.Usage) string {
	if u.CostUSD <= 0 {
		if u.UnknownModel {
			return "$0.00~"
		}
		return "$0.00"
	}
	return asFormatCost(u)
}

// ovShareText is "Claude Code 4" or "Claude Code $3.75".
func ovShareText(sh agentShare, lang string, cost bool) string {
	if cost {
		return asAgentLabel(lang, sh.Agent) + " " + ovFormatSpend(sh.Usage)
	}
	return asAgentLabel(lang, sh.Agent) + " " + strconv.Itoa(sh.Sessions)
}

// ovSplitLabel is the accessible name of the split bar: "Claude Code 4, Codex 1".
func ovSplitLabel(shares []agentShare, lang string, cost bool) string {
	parts := make([]string, 0, len(shares))
	for _, sh := range shares {
		parts = append(parts, ovShareText(sh, lang, cost))
	}
	return strings.Join(parts, ", ")
}

// ovSegment is one rect of the split bar, in viewBox units (0..100).
type ovSegment struct{ Agent, X, W string }

// ovSplitSegments turns the shares into proportional rects separated by a
// 1-unit gap; nil when the total is zero (nothing to split).
func ovSplitSegments(shares []agentShare, cost bool) []ovSegment {
	val := func(sh agentShare) float64 {
		if cost {
			return sh.Usage.CostUSD
		}
		return float64(sh.Sessions)
	}
	total, n := 0.0, 0
	for _, sh := range shares {
		if v := val(sh); v > 0 {
			total += v
			n++
		}
	}
	if total <= 0 {
		return nil
	}
	width := 100 - float64(n-1)
	out := make([]ovSegment, 0, n)
	x := 0.0
	for _, sh := range shares {
		v := val(sh)
		if v <= 0 {
			continue
		}
		w := width * v / total
		out = append(out, ovSegment{Agent: sh.Agent, X: strconv.FormatFloat(x, 'f', 2, 64), W: strconv.FormatFloat(w, 'f', 2, 64)})
		x += w + 1
	}
	return out
}
