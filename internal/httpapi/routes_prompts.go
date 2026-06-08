package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func promptsRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/prompts", handlePromptsList(c))
	mux.HandleFunc("GET /api/prompts/search", handlePromptsSearch(c))
}

func handlePromptsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		limit := parseLimit(r.URL.Query(), 100, 1, 500)
		items, err := services.PromptsList(c.RoDB, services.PromptsListParams{Project: project, Limit: limit})
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
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
		limit := parseLimit(r.URL.Query(), 50, 1, 200)
		items, err := services.PromptsSearch(c.RoDB, rawQ, limit)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Items []services.PromptWithSnippet `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: items})
	}
}
