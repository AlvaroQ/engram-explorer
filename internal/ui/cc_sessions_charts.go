package ui

// CCUsageChartsProps are the props passed to the cc-usage-charts React island.
// Only localized labels are passed; the island fetches its data from
// /api/cc-sessions/stats on its own (server-cached).
type CCUsageChartsProps struct {
	Labels CCUsageLabels `json:"labels"`
}

// CCUsageLabels mirrors the CCUsageLabels interface in the island component.
type CCUsageLabels struct {
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
}

// ccUsageChartsProps builds the island props with labels localized for lang.
func ccUsageChartsProps(lang string) CCUsageChartsProps {
	return CCUsageChartsProps{
		Labels: CCUsageLabels{
			CostByProject:    T(lang, "ccSessions.charts.costByProject"),
			TokensByModel:    T(lang, "ccSessions.charts.tokensByModel"),
			OverTime:         T(lang, "ccSessions.charts.overTime"),
			TopSessions:      T(lang, "ccSessions.charts.topSessions"),
			Sessions:         T(lang, "ccSessions.charts.sessions"),
			Tokens:           T(lang, "ccSessions.charts.tokens"),
			Cost:             T(lang, "ccSessions.charts.cost"),
			Loading:          T(lang, "ccSessions.charts.loading"),
			Empty:            T(lang, "ccSessions.charts.empty"),
			Error:            T(lang, "ccSessions.charts.error"),
			Others:           T(lang, "ccSessions.charts.others"),
			Untitled:         T(lang, "ccSessions.charts.untitled"),
			CostEstimateNote: T(lang, "ccSessions.charts.costEstimateNote"),
		},
	}
}
