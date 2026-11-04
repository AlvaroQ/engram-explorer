package ui

// AgentUsageChartsProps are the props of the agent-usage-charts React island.
// Only localized labels are passed; the island fetches its data from
// /api/agent-sessions/stats on its own (server-cached by fingerprint).
type AgentUsageChartsProps struct {
	Labels AgentUsageLabels  `json:"labels"`
	Agents map[string]string `json:"agents"` // agent kind → display name
}

// AgentUsageLabels mirrors the AgentUsageLabels interface of the island.
type AgentUsageLabels struct {
	CostByAgent      string `json:"costByAgent"`
	CostByProject    string `json:"costByProject"`
	TokensByModel    string `json:"tokensByModel"`
	OverTime         string `json:"overTime"`
	TopSessions      string `json:"topSessions"`
	Sessions         string `json:"sessions"`
	Tokens           string `json:"tokens"`
	Cost             string `json:"cost"`
	Loading          string `json:"loading"`
	Empty            string `json:"empty"`
	Error            string `json:"error"`
	Others           string `json:"others"`
	Untitled         string `json:"untitled"`
	CostEstimateNote string `json:"costEstimateNote"`
	TokenNote        string `json:"tokenNote"`
	SourceError      string `json:"sourceError"` // "{{source}}" / "{{agent}}" placeholders
}

// agentUsageChartsProps builds the island props localized for lang.
func agentUsageChartsProps(lang string) AgentUsageChartsProps {
	k := func(key string) string { return T(lang, "agentSessions.charts."+key) }
	return AgentUsageChartsProps{
		Labels: AgentUsageLabels{
			CostByAgent: k("costByAgent"), CostByProject: k("costByProject"), TokensByModel: k("tokensByModel"),
			OverTime: k("overTime"), TopSessions: k("topSessions"), Sessions: k("sessions"), Tokens: k("tokens"),
			Cost: k("cost"), Loading: k("loading"), Empty: k("empty"), Error: k("error"), Others: k("others"),
			Untitled: k("untitled"), CostEstimateNote: k("costEstimateNote"),
			TokenNote:   T(lang, "agentSessions.tokenNote"),
			SourceError: T(lang, "agentSessions.sourceError"),
		},
		Agents: map[string]string{
			"claude-code": asAgentLabel(lang, "claude-code"),
			"codex":       asAgentLabel(lang, "codex"),
		},
	}
}
