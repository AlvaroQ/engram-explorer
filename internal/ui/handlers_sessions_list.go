package ui

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// handleSessionsListPage serves GET /sessions.
// Full page on direct GET; partial content when HX-Request: true.
// Supports ?project= (repeatable) and ?has_summary= filters with cursor pagination.
func handleSessionsListPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		params := sessionListParamsFromQuery(r)

		result, err := loadSessions(d, params)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load sessions: "+err.Error()))
			return
		}

		if IsHTMX(r) {
			render(w, r, SessionsListPartial(result, params, lang))
		} else {
			renderDeps(w, r, d, SessionsListPage(result, params, lang, theme))
		}
	}
}

// handleSessionsListPartial serves GET /sessions/list.
// Always returns the bare list content (HTMX swap target for filters and load-more).
func handleSessionsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		params := sessionListParamsFromQuery(r)

		result, err := loadSessions(d, params)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load sessions: "+err.Error()))
			return
		}

		// append=true signals load-more: only emit the new rows (no container wrapper).
		if r.URL.Query().Get("append") == "true" {
			render(w, r, SessionsRowsPartial(result, params, lang))
		} else {
			render(w, r, SessionsListBodyPartial(result, params, lang))
		}
	}
}

// sessionListParamsFromQuery extracts SessionListParams from the request query string.
// Supports repeated ?project=a&project=b for multi-project filtering.
func sessionListParamsFromQuery(r *http.Request) services.SessionListParams {
	q := r.URL.Query()

	projects := q["project"]
	// Drop empty strings from the list.
	filtered := projects[:0]
	for _, p := range projects {
		if p != "" {
			filtered = append(filtered, p)
		}
	}

	var hasSummary *bool
	if hs := q.Get("has_summary"); hs != "" {
		v := hs == "true"
		hasSummary = &v
	}

	return services.SessionListParams{
		Projects:   filtered,
		HasSummary: hasSummary,
		Cursor:     q.Get("cursor"),
		Limit:      50,
	}
}

// loadSessions fetches the session list via services.SessionsList.
// When RoDB is nil, returns an empty result so handlers render an empty state.
func loadSessions(d Deps, params services.SessionListParams) (services.SessionListResult, error) {
	if d.RoDB == nil {
		return services.SessionListResult{Items: []services.SessionListItem{}}, nil
	}
	return services.SessionsList(d.RoDB, params)
}
