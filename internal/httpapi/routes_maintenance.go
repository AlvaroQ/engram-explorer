package httpapi

import (
	"net/http"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// maintenanceRoutes registers the JSON Maintenance checks. Reads use the
// read-only pool; the bulk delete is guarded (503 RW_UNAVAILABLE in read-only
// mode) and rate limited like the other write routes.
func maintenanceRoutes(mux routeMux, c *Container) {
	writeRL := newRateLimiter(time.Minute, writeRateLimit)
	mux.HandleFunc("GET /api/maintenance/empty-sessions", handleEmptySessionsList(c))
	mux.Handle("POST /api/maintenance/empty-sessions/delete",
		withRateLimit(writeRL, c.Config.ExposeDetails, handleEmptySessionsDelete(c)))
}

// handleEmptySessionsList serves GET /api/maintenance/empty-sessions.
func handleEmptySessionsList(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep, err := services.ListEmptySessions(c.ro(r), services.EmptySessionsDefaultLimit)
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, rep)
	}
}

// handleEmptySessionsDelete serves POST /api/maintenance/empty-sessions/delete.
// It takes no body: the service deletes, in one transaction, the sessions that
// are still empty at that moment and returns how many it removed.
func handleEmptySessionsDelete(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guardRWDB(w, r, c) {
			return
		}
		res, err := services.DeleteEmptySessions(r.Context(), c.rw(r))
		if err != nil {
			writeDBError(w, err, c.Config.ExposeDetails)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}
