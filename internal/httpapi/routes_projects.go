package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func projectsRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/projects", handleProjectsList(c))
	mux.HandleFunc("GET /api/projects/{project}/overview", handleProjectsGetOverview(c))
	mux.HandleFunc("GET /api/topics", handleTopicsList(c))
}

func handleProjectsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := services.ProjectsList(c.RoDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Items []services.ProjectStats `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: items})
	}
}

func handleProjectsGetOverview(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		overview, err := services.ProjectsGetOverview(c.RoDB, project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		if overview == nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project "+project+" not found", nil, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, overview)
	}
}

func handleTopicsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		items, err := services.TopicsList(c.RoDB, services.TopicsListParams{Project: project})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error", err.Error(), c.Config.ExposeDetails)
			return
		}
		type resp struct {
			Items []services.TopicRow `json:"items"`
		}
		writeJSON(w, http.StatusOK, resp{Items: items})
	}
}
