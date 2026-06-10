package ui

import (
	"net/http"
	"net/url"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ---------------------------------------------------------------------------
// Overview handler
// ---------------------------------------------------------------------------

// handleCCOverviewPage serves GET /cc-overview — the usage charts only (no
// project filter, no sessions list). The cc-usage-charts island fetches its own
// data from /api/cc-sessions/stats, so this handler needs no service call.
func handleCCOverviewPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		render(w, r, CCOverviewPage(lang, theme))
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
		params := ccsListParamsFromQuery(r)

		reader := services.DiskProjectsReader{BaseDir: d.Config.ClaudeProjectsDir}
		result, err := services.CCSessionsList(reader, params)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load Claude Code sessions: "+err.Error()))
			return
		}

		if IsHTMX(r) {
			render(w, r, CCSListPartial(result, params, lang))
		} else {
			render(w, r, CCSListPage(result, params, lang, theme))
		}
	}
}

// handleCCSessionsListPartial serves GET /cc-sessions/list.
// Always returns the bare list content for HTMX swaps.
func handleCCSessionsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		params := ccsListParamsFromQuery(r)

		reader := services.DiskProjectsReader{BaseDir: d.Config.ClaudeProjectsDir}
		result, err := services.CCSessionsList(reader, params)
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
		Cursor:  q.Get("cursor"),
		Limit:   50,
	}
}

// ---------------------------------------------------------------------------
// Detail handler
// ---------------------------------------------------------------------------

// handleCCSessionDetailPage serves GET /cc-sessions/{project}/{id}.
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

		reader := services.DiskProjectsReader{BaseDir: d.Config.ClaudeProjectsDir}
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
			render(w, r, CCSDetailPage(detail, lang, theme))
		}
	}
}
