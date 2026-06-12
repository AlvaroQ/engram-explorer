package ui

import (
	"net/http"
	"net/url"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ccsAccountSources returns the enabled CC account sources from d.CCAccountSources,
// falling back to a single DiskProjectsReader for the configured ClaudeDir when the
// callback is nil (legacy/test paths that build Deps without a ProfileStore).
func ccsAccountSources(d Deps) []services.CCAccountSource {
	if d.CCAccountSources != nil {
		return d.CCAccountSources()
	}
	return []services.CCAccountSource{
		{
			ID:     "default",
			Label:  "Personal",
			Reader: services.DiskProjectsReader{BaseDir: d.Paths.ClaudeDir()},
		},
	}
}

// ---------------------------------------------------------------------------
// Overview handler
// ---------------------------------------------------------------------------

// ccOverviewData holds the data passed to the overview page and partial.
type ccOverviewData struct {
	Multi *services.CCStatsMultiResult
}

// handleCCOverviewPage serves GET /cc-overview.
// Dispatches on ?tab= to render the Overview (charts) or Sessions tab.
// The cc-usage-charts island fetches its own data from /api/cc-sessions/stats;
// the server-rendered breakdown below the charts is computed via CCSessionsStatsMulti.
func handleCCOverviewPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		sidebarState := sidebarStateForRequest(r)
		tab := r.URL.Query().Get("tab")

		switch tab {
		case "sessions":
			params := ccsListParamsFromQuery(r)
			sources := ccsAccountSources(d)
			result, err := services.CCSessionsListMulti(sources, params)
			if err != nil {
				render(w, r, ErrorPartial("Failed to load Claude Code sessions: "+err.Error()))
				return
			}
			if IsHTMX(r) {
				render(w, r, CCSessionsTabPartial(result, params, lang))
			} else {
				renderDeps(w, r, d, CCSessionsTabPage(result, params, lang, theme, sidebarState))
			}

		default:
			sources := ccsAccountSources(d)
			multi, err := services.CCSessionsStatsMulti(sources)
			if err != nil {
				render(w, r, ErrorPartial("Failed to load Claude Code stats: "+err.Error()))
				return
			}
			renderDeps(w, r, d, CCOverviewPage(multi, lang, theme, sidebarState))
		}
	}
}

// ---------------------------------------------------------------------------
// List handlers
// ---------------------------------------------------------------------------

// handleCCSessionsListPage serves GET /cc-sessions.
// Full page on direct GET; partial when HX-Request: true.
func handleCCSessionsListPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		sidebarState := sidebarStateForRequest(r)
		params := ccsListParamsFromQuery(r)

		sources := ccsAccountSources(d)
		result, err := services.CCSessionsListMulti(sources, params)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load Claude Code sessions: "+err.Error()))
			return
		}

		if IsHTMX(r) {
			render(w, r, CCSListPartial(result, params, lang))
		} else {
			renderDeps(w, r, d, CCSListPage(result, params, lang, theme, sidebarState))
		}
	}
}

// handleCCSessionsListPartial serves GET /cc-sessions/list.
// Always returns the bare list content for HTMX swaps.
func handleCCSessionsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		params := ccsListParamsFromQuery(r)

		sources := ccsAccountSources(d)
		result, err := services.CCSessionsListMulti(sources, params)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load Claude Code sessions: "+err.Error()))
			return
		}

		if r.URL.Query().Get("append") == "true" {
			render(w, r, CCSRowsPartial(result, params, lang))
		} else {
			render(w, r, CCSListBodyPartial(result, params, lang))
		}
	}
}

// ccsListParamsFromQuery extracts CCSessionListParams from the request query string.
func ccsListParamsFromQuery(r *http.Request) services.CCSessionListParams {
	q := r.URL.Query()
	return services.CCSessionListParams{
		Project: q.Get("project"),
		Account: q.Get("account"),
		Cursor:  q.Get("cursor"),
		Limit:   50,
	}
}

// ---------------------------------------------------------------------------
// Detail handler
// ---------------------------------------------------------------------------

// handleCCSessionDetailPage serves GET /cc-sessions/{project}/{id}.
// The optional ?account=<id> query parameter selects which account's reader to
// use. When absent (or unresolvable), the first enabled source is used.
func handleCCSessionDetailPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawProject := r.PathValue("project")
		id := r.PathValue("id")

		// URL-decode the project folder name.
		projectFolder, err := url.PathUnescape(rawProject)
		if err != nil {
			http.Error(w, "bad project path", http.StatusBadRequest)
			return
		}

		// Security: validate path components before building a filesystem path.
		if err := services.ValidateCCSessionPath(projectFolder, id); err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		lang := langForRequest(r)
		theme := themeForRequest(r)
		sidebarState := sidebarStateForRequest(r)

		// Resolve the correct reader: prefer the account matching ?account=<id>;
		// fall back to the first available source when the param is absent or
		// the account is not found among enabled sources.
		reader := ccsResolveReader(d, r.URL.Query().Get("account"))

		detail, err := services.CCSessionGetDetail(reader, projectFolder, id)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load session: "+err.Error()))
			return
		}
		if detail == nil {
			http.NotFound(w, r)
			return
		}

		if IsHTMX(r) {
			render(w, r, CCSDetailPartial(detail, lang))
		} else {
			renderDeps(w, r, d, CCSDetailPage(detail, lang, theme, sidebarState))
		}
	}
}

// ccsResolveReader finds the CCProjectsReader for the given accountID among
// enabled sources. When accountID is empty or not found, it returns the reader
// of the first available source. The Deps fallback (nil CCAccountSources) is
// handled by ccsAccountSources, so this function is always safe to call.
func ccsResolveReader(d Deps, accountID string) services.CCProjectsReader {
	sources := ccsAccountSources(d)
	if len(sources) == 0 {
		// Should not happen — ccsAccountSources always returns ≥1 source.
		return services.DiskProjectsReader{BaseDir: d.Paths.ClaudeDir()}
	}
	if accountID != "" {
		for _, s := range sources {
			if s.ID == accountID {
				return s.Reader
			}
		}
	}
	return sources[0].Reader
}
