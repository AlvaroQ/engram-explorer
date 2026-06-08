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

		limit := parseLimit(q, 50, 1, 500)

		params := services.ObservationListParams{
			Projects:       projects,
			Types:          types,
			ToolNames:      toolNames,
			Scope:          q.Get("scope"),
			TopicKey:       q.Get("topic_key"),
			Q:              q.Get("q"),
			From:           q.Get("from"),
			To:             q.Get("to"),
			IncludeDeleted: parseBool(q.Get("include_deleted")),
			OnlyDeleted:    parseBool(q.Get("only_deleted")),
			Cursor:         q.Get("cursor"),
			Limit:          limit,
		}

		items, nextCursor, err := services.ObservationsList(c.RoDB, params)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
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
			writeDBError(w, err, c.Config.ExposeDetails)
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
		limit := parseLimit(r.URL.Query(), 50, 1, 200)
		items, err := services.ObservationsSearch(c.RoDB, rawQ, limit)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
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
			writeDBError(w, err, c.Config.ExposeDetails)
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
