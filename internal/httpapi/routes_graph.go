package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func graphRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/graph", handleGraphBuild(c))
}

func handleGraphBuild(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		// mode is reserved for future galaxy/supernode LOD — parsed-and-ignored.
		result, err := services.GraphBuild(c.RoDB, project, 0)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
