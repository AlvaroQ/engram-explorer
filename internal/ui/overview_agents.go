package ui

import (
	"context"
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
