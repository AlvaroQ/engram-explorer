package httpapi

import (
	"net/http"
	"strconv"

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

		limit := 50
		if l := q.Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 500 {
				limit = n
			}
		}

		var hasSummary *bool
		if hs := q.Get("has_summary"); hs != "" {
			v := hs == "true" || hs == "1"
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
			Enrich:     q.Get("enrich") == "true" || q.Get("enrich") == "1",
		}

		result, err := services.SessionsList(c.RoDB, params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
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
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
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
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
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
