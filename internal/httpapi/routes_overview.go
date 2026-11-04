package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

func overviewRoutes(mux routeMux, c *Container) {
	mux.HandleFunc("GET /api/overview", handleOverview(c))
	mux.HandleFunc("GET /api/activity", handleActivity(c))
}

// handleActivity serves GET /api/activity?range=7d|30d|90d (per-day, per-project
// observation counts). Mirrors Node's activity route: range defaults to "30d"
// and an unknown range is a 400 BAD_INPUT.
func handleActivity(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rng := r.URL.Query().Get("range")
		if rng == "" {
			rng = "30d"
		}
		if !services.ValidActivityRange(rng) {
			writeError(w, http.StatusBadRequest, "BAD_INPUT",
				"invalid range: "+rng+". Allowed: 7d, 30d, 90d", "", c.Config.ExposeDetails)
			return
		}
		resp, err := services.ActivityByProject(c.ro(r), rng)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleOverview(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Served by the live providers.StatsSource capability.
		stats := c.statsSource(r)
		if stats == nil {
			c.writeModuleDisabled(w, r)
			return
		}
		resp, err := stats.Stats(r.Context())
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
