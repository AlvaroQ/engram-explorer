package ui

import (
	"net/http"
	"sort"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// TypeBreakdownEntry mirrors the island's TypeBreakdownEntry interface.
type TypeBreakdownEntry struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// TypeBreakdownIslandProps is the props shape for the type-breakdown island.
type TypeBreakdownIslandProps struct {
	ByType []TypeBreakdownEntry `json:"by_type"`
}

// BrainPreviewCardIslandProps are the props for the brain-preview-card island.
type BrainPreviewCardIslandProps struct {
	Title   string `json:"title"`
	ColorBy string `json:"colorBy"`
}

// overviewData bundles data needed to render the overview page.
type overviewData struct {
	Overview services.OverviewResponse
	Issues   services.SyncIssuesResponse
}

// loadOverview fetches all overview data; returns empty structs when RoDB is nil.
func loadOverview(d Deps) (overviewData, error) {
	if d.RoDB == nil {
		return overviewData{
			Overview: services.OverviewResponse{
				ByType:             []services.TypeCount{},
				Activity30d:        []services.ActivityDay{},
				RecentObservations: []services.OverviewRecentObs{},
			},
			Issues: services.SyncIssuesResponse{
				Issues: []services.SyncIssue{},
			},
		}, nil
	}

	overview, err := services.OverviewBuild(d.RoDB)
	if err != nil {
		return overviewData{}, err
	}

	// daemon availability is unknown here; use false as a safe default.
	// The issues table is informational — a false negative is preferable to a
	// missing page.
	issues, err := services.SyncComputeIssues(d.RoDB, false)
	if err != nil {
		// issues are best-effort — render with empty issues if query fails.
		issues = services.SyncIssuesResponse{Issues: []services.SyncIssue{}}
	}

	return overviewData{Overview: *overview, Issues: issues}, nil
}

// handleOverviewPage serves GET / (the home/overview page).
// When ActiveCount is non-nil and returns 0, the onboarding zero-state is
// rendered instead of the regular overview.
func handleOverviewPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)

		// Onboarding branch: zero active providers → show onboarding page.
		if d.ActiveCount != nil && d.ActiveCount() == 0 {
			od := buildOnboardingData(d, lang, theme)
			if IsHTMX(r) {
				renderDeps(w, r, d, OnboardingPartial(od))
			} else {
				renderDeps(w, r, d, OnboardingPage(od))
			}
			return
		}

		data, err := loadOverview(d)
		if err != nil {
			renderDeps(w, r, d, ErrorPartial("Failed to load overview: "+err.Error()))
			return
		}

		if IsHTMX(r) {
			render(w, r, OverviewPartial(data.Overview, data.Issues, lang))
		} else {
			renderDeps(w, r, d, OverviewPage(data.Overview, data.Issues, lang, theme))
		}
	}
}

// --- Helper types and functions used by overview.templ ---

// issueGroup groups SyncIssues by project for the issues table.
type issueGroup struct {
	Key         string
	IsGlobal    bool
	Issues      []services.SyncIssue
	MaxSeverity services.SyncIssueSeverity
}

var severityRank = map[services.SyncIssueSeverity]int{
	services.SyncIssueSeverityHigh:   3,
	services.SyncIssueSeverityMedium: 2,
	services.SyncIssueSeverityLow:    1,
	services.SyncIssueSeverityInfo:   0,
}

// groupSyncIssues groups issues by project key, sorted by max severity.
func groupSyncIssues(issues []services.SyncIssue) []issueGroup {
	buckets := make(map[string]*issueGroup)
	const globalKey = "__global__"

	for _, issue := range issues {
		key := globalKey
		if issue.Project != nil && *issue.Project != "" {
			key = *issue.Project
		}
		g, ok := buckets[key]
		if !ok {
			g = &issueGroup{
				Key:         key,
				IsGlobal:    key == globalKey,
				MaxSeverity: issue.Severity,
			}
			if key != globalKey {
				g.Key = *issue.Project
			}
			buckets[key] = g
		}
		g.Issues = append(g.Issues, issue)
		if severityRank[issue.Severity] > severityRank[g.MaxSeverity] {
			g.MaxSeverity = issue.Severity
		}
	}

	result := make([]issueGroup, 0, len(buckets))
	for _, g := range buckets {
		result = append(result, *g)
	}

	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.IsGlobal != b.IsGlobal {
			return a.IsGlobal
		}
		ra, rb := severityRank[a.MaxSeverity], severityRank[b.MaxSeverity]
		if ra != rb {
			return ra > rb
		}
		if len(a.Issues) != len(b.Issues) {
			return len(a.Issues) > len(b.Issues)
		}
		return a.Key < b.Key
	})

	return result
}

// overviewBrokenTone returns the CSS tone class for the broken count.
func overviewBrokenTone(count int64) string {
	if count > 0 {
		return "text-fail"
	}
	return "text-fg"
}

// overviewPendingTone returns the CSS tone class for pending mutations.
func overviewPendingTone(count int64) string {
	if count > 100 {
		return "text-warn"
	}
	return "text-fg"
}

// overviewSeverityBadgeClass returns a badge CSS class for a severity level.
func overviewSeverityBadgeClass(severity string) string {
	switch severity {
	case "HIGH":
		return "badge badge-fail"
	case "MEDIUM":
		return "badge badge-warn"
	case "LOW":
		return "badge badge-accent"
	default:
		return "badge badge-neutral"
	}
}

// overviewIssueCountLabel returns "1 issue" or "N issues".
func overviewIssueCountLabel(count int, lang string) string {
	if count == 1 {
		return T(lang, "overview.issues.group.countOne", "count", "1")
	}
	return T(lang, "overview.issues.group.count", "count", itoa(count))
}

// itoa converts int to string without importing strconv in the templ file.
func itoa(n int) string {
	return intToStr(n)
}

// intToStr converts int to decimal string.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// derefStr dereferences a *string; returns fallback if nil.
func derefStr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

// toTypeBreakdownEntries converts services.TypeCount slice to island props.
func toTypeBreakdownEntries(tc []services.TypeCount) []TypeBreakdownEntry {
	out := make([]TypeBreakdownEntry, len(tc))
	for i, t := range tc {
		out[i] = TypeBreakdownEntry{Type: t.Type, Count: int(t.Count)}
	}
	return out
}
