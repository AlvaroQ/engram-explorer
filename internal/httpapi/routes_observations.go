package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// observationsRoutes registers all /api/observations/* routes on mux.
// IMPORTANT: /api/observations/types and /api/observations/search must be
// registered BEFORE /api/observations/{id} so that the static paths take
// precedence. Go 1.22+ ServeMux matches most-specific path first, but explicit
// order avoids any ambiguity.
func observationsRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/observations", handleObservationsList(c))
	mux.HandleFunc("GET /api/observations/types", handleObservationsTypes(c))
	mux.HandleFunc("GET /api/observations/search", handleObservationsSearch(c))
	mux.HandleFunc("GET /api/observations/{id}", handleObservationsGetByID(c))
}

// expandList splits a comma-separated value or accepts multiple occurrences.
// Mirrors the Node expandList() helper in routes/observations.ts.
func expandList(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func handleObservationsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		projects := expandList(q["project"])
		types := expandList(q["type"])
		toolNames := expandList(q["tool_name"])

		limit := 50
		if l := q.Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 500 {
				limit = n
			}
		}

		params := services.ObservationListParams{
			Projects:       projects,
			Types:          types,
			ToolNames:      toolNames,
			Scope:          q.Get("scope"),
			TopicKey:       q.Get("topic_key"),
			Q:              q.Get("q"),
			From:           q.Get("from"),
			To:             q.Get("to"),
			IncludeDeleted: q.Get("include_deleted") == "true" || q.Get("include_deleted") == "1",
			OnlyDeleted:    q.Get("only_deleted") == "true" || q.Get("only_deleted") == "1",
			Cursor:         q.Get("cursor"),
			Limit:          limit,
		}

		items, nextCursor, err := services.ObservationsList(c.RoDB, params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		if items == nil {
			items = []services.ObservationRow{}
		}

		type resp struct {
			Items      []services.ObservationRow `json:"items"`
			NextCursor *string                   `json:"nextCursor"`
			Total      *int                      `json:"total"` // always null (Node parity)
		}
		writeJSON(w, http.StatusOK, resp{Items: items, NextCursor: nextCursor, Total: nil})
	}
}

func handleObservationsTypes(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		types, err := services.ObservationsListTypes(c.RoDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Items []string `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: types})
	}
}

func handleObservationsSearch(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawQ := r.URL.Query().Get("q")
		if rawQ == "" {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "q is required", nil, c.Config.ExposeDetails)
			return
		}
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				if n < 1 {
					n = 1
				}
				if n > 200 {
					n = 200
				}
				limit = n
			}
		}
		items, err := services.ObservationsSearch(c.RoDB, rawQ, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}

		type resp struct {
			Items []services.ObservationWithSnippet `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: items})
	}
}

func handleObservationsGetByID(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_INPUT", "id must be an integer", nil, c.Config.ExposeDetails)
			return
		}

		obs, revisions, err := services.ObservationsGetByID(c.RoDB, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		if obs == nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "observation "+idStr+" not found", nil, c.Config.ExposeDetails)
			return
		}

		type resp struct {
			Observation *services.ObservationRow  `json:"observation"`
			Revisions   []services.ObservationRow `json:"revisions"`
		}
		writeJSON(w, http.StatusOK, resp{Observation: obs, Revisions: revisions})
	}
}
