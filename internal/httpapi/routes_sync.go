package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func syncRoutes(mux routeMux, c *Container) {
	mux.HandleFunc("GET /api/sync/projects", handleSyncProjectsList(c))
	// /api/sync/issues must be registered before /api/sync/{project} to avoid
	// "issues" being captured as a project name.
	mux.HandleFunc("GET /api/sync/issues", handleSyncIssues(c))
	mux.HandleFunc("GET /api/sync/{project}", handleSyncProjectDetail(c))
}

func handleSyncProjectsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := services.SyncListProjects(c.ro(r))
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleSyncProjectDetail(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		detail, err := services.SyncGetProjectDetail(c.ro(r), project)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		if detail == nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project "+project+" not found", nil, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

func handleSyncIssues(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dc := daemon.New(c.Config.DaemonBaseURL, c.Config.DaemonTimeoutMs)
		pingResult := dc.FetchJSON("/health")
		daemonAvailable := pingResult.OK

		result, err := services.SyncComputeIssues(c.ro(r), daemonAvailable)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
