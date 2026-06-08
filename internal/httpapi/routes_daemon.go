package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
)

func daemonRoutes(mux *http.ServeMux, c *Container) {
	dc := daemon.New(c.Config.DaemonBaseURL, c.Config.DaemonTimeoutMs)
	mux.HandleFunc("GET /api/daemon/health", handleDaemonProxy(c, dc, "/health"))
	mux.HandleFunc("GET /api/daemon/sync-status", handleDaemonProxy(c, dc, "/sync/status"))
	mux.HandleFunc("GET /api/daemon/stats", handleDaemonProxy(c, dc, "/stats"))
}

// handleDaemonProxy returns a handler that proxies a single daemon path.
// On success it passes through the daemon's JSON body verbatim (preserving key
// order). On failure it returns a 503 DAEMON_UNAVAILABLE envelope.
func handleDaemonProxy(c *Container, dc *daemon.Client, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := dc.FetchJSON(path)
		if !result.OK {
			type errEnv struct {
				Code    string              `json:"code"`
				Message string              `json:"message"`
				Details *daemon.DaemonError `json:"details,omitempty"`
			}
			type env struct {
				Error errEnv `json:"error"`
			}
			msg := "daemon " + path + ": " + result.Error.Message
			var details *daemon.DaemonError
			if c.Config.ExposeDetails {
				details = result.Error
			}
			writeJSON(w, http.StatusServiceUnavailable, env{
				Error: errEnv{
					Code:    "DAEMON_UNAVAILABLE",
					Message: msg,
					Details: details,
				},
			})
			return
		}
		writeRawJSON(w, http.StatusOK, result.Data)
	}
}
