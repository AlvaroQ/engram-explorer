package httpapi

import (
	"net/http"
	"strconv"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func promptsRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/prompts", handlePromptsList(c))
	mux.HandleFunc("GET /api/prompts/search", handlePromptsSearch(c))
}

func handlePromptsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				if n < 1 {
					n = 1
				}
				if n > 500 {
					n = 500
				}
				limit = n
			}
		}
		items, err := services.PromptsList(c.RoDB, services.PromptsListParams{Project: project, Limit: limit})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Items []services.PromptRow `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: items})
	}
}

func handlePromptsSearch(c *Container) http.HandlerFunc {
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
		items, err := services.PromptsSearch(c.RoDB, rawQ, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Items []services.PromptWithSnippet `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: items})
	}
}
