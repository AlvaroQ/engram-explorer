package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// observationListParamsFromQuery parses the 7 filter params + cursor/limit from
// the request query string. Mirrors the React page's ObservationListParams shape.
func observationListParamsFromQuery(r *http.Request) services.ObservationListParams {
	q := r.URL.Query()

	projects := dropEmpty(q["project"])
	types := dropEmpty(q["type"])
	toolNames := dropEmpty(q["tool_name"])

	limit := 50
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	deleted := q.Get("deleted")
	includeDeleted := deleted == "all"
	onlyDeleted := deleted == "only"

	return services.ObservationListParams{
		Projects:       projects,
		Types:          types,
		ToolNames:      toolNames,
		Scope:          q.Get("scope"),
		TopicKey:       q.Get("topic_key"),
		Q:              q.Get("q"),
		IncludeDeleted: includeDeleted,
		OnlyDeleted:    onlyDeleted,
		Cursor:         q.Get("cursor"),
		Limit:          limit,
	}
}

// dropEmpty filters out empty strings from a slice in-place.
func dropEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// loadObservations fetches observations via the service. Returns empty result
// when RoDB is nil.
func loadObservations(d Deps, p services.ObservationListParams) ([]services.ObservationRow, *string, error) {
	if d.RoDB == nil {
		return []services.ObservationRow{}, nil, nil
	}
	return services.ObservationsList(d.RoDB, p)
}

// loadProjectNames fetches the distinct project names for the filter dropdown.
// Returns an empty slice when RoDB is nil or on error (graceful degradation).
func loadProjectNames(d Deps) []string {
	if d.RoDB == nil {
		return []string{}
	}
	stats, err := services.ProjectsList(d.RoDB)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(stats))
	for _, s := range stats {
		if s.Project != "" {
			names = append(names, s.Project)
		}
	}
	return names
}

// handleObservationsPage serves GET /observations (all three Memory tab views).
// Dispatches on ?view= to render Threads or Conversations sub-views.
// Full page on direct GET; partial content when HX-Request: true.
func handleObservationsPage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		theme := themeForRequest(r)
		sidebarState := sidebarStateForRequest(r)
		view := r.URL.Query().Get("view")

		switch view {
		case "threads":
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			topics, err := loadTopics(r, d, q)
			if err != nil {
				render(w, r, ErrorPartial("Failed to load topics: "+err.Error()))
				return
			}
			if IsHTMX(r) {
				render(w, r, MemoryThreadsPartial(topics, q, lang))
			} else {
				renderDeps(w, r, d, MemoryThreadsPage(topics, q, lang, theme, sidebarState))
			}

		case "conversations":
			q := r.URL.Query().Get("q")
			items, searchItems, err := loadPrompts(d, q)
			if err != nil {
				render(w, r, ErrorPartial("Failed to load prompts: "+err.Error()))
				return
			}
			if IsHTMX(r) {
				render(w, r, MemoryConversationsPartial(items, searchItems, q, lang))
			} else {
				renderDeps(w, r, d, MemoryConversationsPage(items, searchItems, q, lang, theme, sidebarState))
			}

		default:
			advanced := d.AdvancedView != nil && d.AdvancedView()
			params := observationListParamsFromQuery(r)
			items, nextCursor, err := loadObservations(d, params)
			if err != nil {
				render(w, r, ErrorPartial("Failed to load observations: "+err.Error()))
				return
			}
			projects := loadProjectNames(d)
			if IsHTMX(r) {
				render(w, r, ObservationsPartial(items, nextCursor, params, projects, lang, advanced))
			} else {
				renderDeps(w, r, d, ObservationsPage(items, nextCursor, params, projects, lang, theme, sidebarState, advanced))
			}
		}
	}
}

// handleObservationsListPartial serves GET /observations/list.
// Always returns the bare observations rows partial (HTMX filter+load-more target).
func handleObservationsListPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)
		// The load-more URL carries the column mode that was in effect when the
		// page rendered; honour it so appended rows match the existing header.
		// Filter-change requests omit it and fall back to the live preference.
		advanced := d.AdvancedView != nil && d.AdvancedView()
		if r.URL.Query().Has("advanced") {
			advanced = r.URL.Query().Get("advanced") == "true"
		}
		params := observationListParamsFromQuery(r)

		items, nextCursor, err := loadObservations(d, params)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load observations: "+err.Error()))
			return
		}

		// append=true signals load-more: only emit new rows (no wrapper or filter controls).
		if r.URL.Query().Get("append") == "true" {
			render(w, r, ObservationsRowsPartial(items, nextCursor, params, lang, advanced))
		} else {
			render(w, r, ObservationsTablePartial(items, nextCursor, params, lang, advanced))
		}
	}
}

// handleObservationDetailPartial serves GET /observations/{id}.
// Returns the observation detail partial for the aside drawer.
func handleObservationDetailPartial(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langForRequest(r)

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			render(w, r, ErrorPartial("Invalid observation id: "+idStr))
			return
		}

		if d.RoDB == nil {
			http.NotFound(w, r)
			return
		}

		obs, revisions, err := services.ObservationsGetByID(d.RoDB, id)
		if err != nil {
			render(w, r, ErrorPartial("Failed to load observation: "+err.Error()))
			return
		}
		if obs == nil {
			http.NotFound(w, r)
			return
		}

		render(w, r, ObservationDetailPartial(obs, revisions, fmt.Sprintf("#%d", obs.ID), lang))
	}
}
