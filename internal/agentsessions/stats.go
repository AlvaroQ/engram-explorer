package agentsessions

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"
)

// TopSessionsLimit caps StatsResult.TopSessions.
const TopSessionsLimit = 10

// UnknownModelKey groups usage that cannot be attributed to a single model.
const UnknownModelKey = "unknown"

// StatsResult aggregates usage across sources. It mirrors the Claude-only
// CCStatsResult (totals, per project/model/day, top sessions) and adds the
// per-agent split.
type StatsResult struct {
	GeneratedAt     time.Time     `json:"generated_at"`
	Sessions        int           `json:"sessions"`
	HasUnknownModel bool          `json:"has_unknown_model"`
	Totals          Usage         `json:"totals"`
	ByAgent         []GroupStat   `json:"by_agent"`
	ByProject       []GroupStat   `json:"by_project"`
	ByModel         []GroupStat   `json:"by_model"`
	OverTime        []DayStat     `json:"over_time"`
	TopSessions     []TopSession  `json:"top_sessions"`
	Errors          []SourceError `json:"errors,omitempty"`
}

// GroupStat is usage aggregated under one key (agent, project or model).
type GroupStat struct {
	Key      string `json:"key"`
	Sessions int    `json:"sessions"`
	Usage    Usage  `json:"usage"`
}

// DayStat is usage per UTC day of StartedAt, with a per-agent split.
type DayStat struct {
	Day      string           `json:"day"` // "2006-01-02"
	Sessions int              `json:"sessions"`
	Usage    Usage            `json:"usage"`
	ByAgent  map[string]Usage `json:"by_agent"`
}

// TopSession carries enough to deep-link to the session detail.
type TopSession struct {
	Source      string    `json:"source"`
	Agent       string    `json:"agent"`
	Key         string    `json:"key"`
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	FirstPrompt string    `json:"first_prompt"` // whitespace-collapsed, max 80 runes
	StartedAt   time.Time `json:"started_at"`
	Usage       Usage     `json:"usage"`
}

type groupAcc map[string]*GroupStat

func (g groupAcc) add(key string, u Usage) {
	e := g[key]
	if e == nil {
		e = &GroupStat{Key: key}
		g[key] = e
	}
	e.Sessions++
	e.Usage.Add(u)
}

func (g groupAcc) sorted(less func(a, b GroupStat) int) []GroupStat {
	out := make([]GroupStat, 0, len(g))
	for _, e := range g {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b GroupStat) int {
		return cmp.Or(less(a, b), cmp.Compare(a.Key, b.Key))
	})
	return out
}

// moreUsage orders by cost desc, then total tokens desc.
func moreUsage(a, b Usage) int {
	return cmp.Or(cmp.Compare(b.CostUSD, a.CostUSD), cmp.Compare(b.Total(), a.Total()))
}

// modelSplit returns the per-model usage of a session (see Session.ModelUsage).
func modelSplit(s Session) map[string]Usage {
	if len(s.ModelUsage) > 0 {
		return s.ModelUsage
	}
	if len(s.Models) == 1 {
		return map[string]Usage{s.Models[0]: s.Usage}
	}
	return map[string]Usage{UnknownModelKey: s.Usage}
}

// Stats aggregates every session matching p's filters across all sources.
// Failing sources are reported in Errors; only ctx cancellation errors out.
func Stats(ctx context.Context, sources []Source, p ListParams) (*StatsResult, error) {
	c, err := Collect(ctx, sources)
	if err != nil {
		return nil, err
	}
	res := &StatsResult{GeneratedAt: time.Now().UTC(), Errors: c.Errors}
	byAgent, byProject, byModel := groupAcc{}, groupAcc{}, groupAcc{}
	byDay := map[string]*DayStat{}
	top := make([]TopSession, 0)

	for _, s := range filter(c.Sessions, p) {
		res.Sessions++
		res.Totals.Add(s.Usage)
		byAgent.add(s.Agent, s.Usage)
		byProject.add(s.Project, s.Usage)
		for m, u := range modelSplit(s) {
			byModel.add(m, u)
		}
		if !s.StartedAt.IsZero() {
			key := s.StartedAt.UTC().Format(time.DateOnly)
			d := byDay[key]
			if d == nil {
				d = &DayStat{Day: key, ByAgent: map[string]Usage{}}
				byDay[key] = d
			}
			d.Sessions++
			d.Usage.Add(s.Usage)
			au := d.ByAgent[s.Agent]
			au.Add(s.Usage)
			d.ByAgent[s.Agent] = au
		}
		top = append(top, TopSession{
			Source: s.Source, Agent: s.Agent, Key: s.Key, ID: s.ID, Project: s.Project,
			FirstPrompt: truncateLabel(s.FirstPrompt, 80), StartedAt: s.StartedAt, Usage: s.Usage,
		})
	}
	res.HasUnknownModel = res.Totals.UnknownModel

	byUsage := func(a, b GroupStat) int { return moreUsage(a.Usage, b.Usage) }
	res.ByAgent = byAgent.sorted(byUsage)
	res.ByProject = byProject.sorted(byUsage)
	res.ByModel = byModel.sorted(func(a, b GroupStat) int { return cmp.Compare(b.Usage.Total(), a.Usage.Total()) })

	res.OverTime = make([]DayStat, 0, len(byDay))
	for _, d := range byDay {
		res.OverTime = append(res.OverTime, *d)
	}
	slices.SortFunc(res.OverTime, func(a, b DayStat) int { return cmp.Compare(a.Day, b.Day) })

	slices.SortFunc(top, func(a, b TopSession) int {
		return cmp.Or(moreUsage(a.Usage, b.Usage), cmp.Compare(a.Source, b.Source), cmp.Compare(a.Key, b.Key))
	})
	res.TopSessions = top[:min(len(top), TopSessionsLimit)]
	return res, nil
}

// truncateLabel collapses whitespace and caps runes with an ellipsis.
func truncateLabel(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
