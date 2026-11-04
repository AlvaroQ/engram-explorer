package httpapi

import (
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
)

func daemonRoutes(mux routeMux, c *Container) {
	dc := daemon.New(c.Config.DaemonBaseURL, c.Config.DaemonTimeoutMs)
	mux.HandleFunc("GET /api/daemon/health", handleDaemonProxy(c, dc, "/health"))
	mux.HandleFunc("GET /api/daemon/sync-status", handleDaemonProxy(c, dc, "/sync/status"))
	mux.HandleFunc("GET /api/daemon/stats", handleDaemonProxy(c, dc, "/stats"))
}

// handleDaemonProxy returns a handler that proxies a single daemon path.
// On success it passes through the daemon's JSON body verbatim (preserving key
// order). On failure it returns a 503 DAEMON_UNAVAILABLE envelope. The daemon
// client error text names the daemon URL and the dial error, so production
// answers a generic message with the daemon error code (NETWORK, TIMEOUT,
// HTTP, PARSE) and logs the text; only development (ExposeDetails) puts the
// text in the message and the client error in details.
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
			msg := "daemon " + path + " unavailable (" + string(result.Error.Code) + ")"
			var details *daemon.DaemonError
			if c.Config.ExposeDetails {
				msg = "daemon " + path + ": " + result.Error.Message
				details = result.Error
			} else if c.Logger != nil {
				c.Logger.WarnContext(r.Context(), "daemon proxy request failed",
					"path", path, "code", result.Error.Code, "err", result.Error.Message)
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
