package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func sessionsRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/sessions", handleSessionsList(c))
	mux.HandleFunc("GET /api/sessions/{id}", handleSessionsGetDetail(c))
	mux.HandleFunc("GET /api/sessions/{id}/events", handleSessionsGetEvents(c))
}

func handleSessionsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		projects := expandList(q["project"])

		limit := parseLimit(q, 50, 1, 500)

		var hasSummary *bool
		if hs := q.Get("has_summary"); hs != "" {
			v := parseBool(hs)
			hasSummary = &v
		}

		params := services.SessionListParams{
			Projects:   projects,
			From:       q.Get("from"),
			To:         q.Get("to"),
			HasSummary: hasSummary,
			Cursor:     q.Get("cursor"),
			Limit:      limit,
			Sort:       q.Get("sort"),
			Enrich:     parseBool(q.Get("enrich")),
		}

		result, err := services.SessionsList(c.RoDB, params)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func handleSessionsGetDetail(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		detail, err := services.SessionsGetDetail(c.RoDB, id)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		if detail == nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session "+id+" not found", nil, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

func handleSessionsGetEvents(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		detail, err := services.SessionsGetDetail(c.RoDB, id)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		if detail == nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "session "+id+" not found", nil, c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Events []services.SessionEvent `json:"events"`
		}
		writeJSON(w, http.StatusOK, resp{Events: detail.Events})
	}
}
