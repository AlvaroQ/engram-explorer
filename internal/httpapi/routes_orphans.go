package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func orphansRoutes(mux *http.ServeMux, c *Container) {
	mux.HandleFunc("GET /api/orphans", handleOrphansList(c))
}

func handleOrphansList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := services.OrphansList(c.RoDB)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
