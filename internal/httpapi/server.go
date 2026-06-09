package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/daemon"
	"github.com/AlvaroQ/engram-explorer/internal/doctor"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// startTime is recorded once so uptime_s can be computed in the health handler.
var startTime = time.Now()

// NewServeMux builds the ServeMux with all registered routes and middleware chain.
// Middleware order (preserved from the original Hono backend during migration):
//  1. Secure headers
//  2. Request logging
//  3. CORS (dev only, allow-list)
//  4. Route dispatch (rate limiters applied per-route in mux registration)
func NewServeMux(c *Container) http.Handler {
	mux := http.NewServeMux()

	// --- Phase 1 routes ---
	mux.HandleFunc("GET /api/health", healthHandler(c))

	// --- Phase 2 part 1 routes ---
	observationsRoutes(mux, c)
	promptsRoutes(mux, c)
	projectsRoutes(mux, c)
	overviewRoutes(mux, c)
	ccSessionsRoutes(mux, c)
	orphansRoutes(mux, c)
	daemonRoutes(mux, c)

	// --- Phase 2 part 2 routes ---
	sessionsRoutes(mux, c)
	syncRoutes(mux, c)
	graphRoutes(mux, c)

	// --- Phase 3 write routes ---
	writeRoutes(mux, c)

	// --- Phase 4 cloud CLI routes ---
	cloudRoutes(mux, c)

	// --- Doctor diagnostics module (templ+HTMX) ---
	doctor.Mount(mux, doctor.Deps{
		RoDB:   c.RoDB,
		RWDB:   c.RWDB,
		Config: c.Config,
	})

	// --- UI module — server-rendered pages (templ+HTMX) ---
	ui.Mount(mux, ui.Deps{
		RoDB:   c.RoDB,
		RWDB:   c.RWDB,
		Config: c.Config,
	})

	// Meta endpoint at /api (was previously at GET /; moved to avoid conflict with
	// the UI root handler GET /{$} registered by ui.Mount).
	mux.HandleFunc("GET /api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name":    "engram-explorer-dashboard",
			"version": "0.1.0",
			"docs":    "/api",
		})
	})

	// Apply middleware stack (outermost = first to run).
	var handler http.Handler = mux
	handler = corsMiddleware(handler)
	handler = requestLogger(c.Logger, handler)
	handler = secureHeaders(handler)

	return handler
}

// healthHandler replicates the Node /api/health shape exactly:
//
//	{
//	  "ok": bool,
//	  "db": { "ok": bool, "path": string },
//	  "daemon": { "ok": bool, "url": string, "error"?: string },
//	  "uptime_s": int
//	}
//
// The daemon ping is a best-effort HTTP call; we omit the full daemon proxy in
// Phase 1 and always report daemon.ok=false with a placeholder error.
func healthHandler(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// DB ping
		dbOk := false
		var dbErr error
		if c.RoDB != nil {
			var n int
			dbErr = c.RoDB.QueryRow("SELECT 1").Scan(&n)
			dbOk = dbErr == nil && n == 1
		}

		dbBody := map[string]any{
			"ok":   dbOk,
			"path": c.Config.EngramDbPath,
		}

		// Daemon ping using the real daemon client.
		dc := daemon.New(c.Config.DaemonBaseURL, c.Config.DaemonTimeoutMs)
		daemonResult := dc.FetchJSON("/health")
		daemonBody := map[string]any{
			"ok":  daemonResult.OK,
			"url": c.Config.DaemonBaseURL,
		}
		if !daemonResult.OK && daemonResult.Error != nil {
			daemonBody["error"] = daemonResult.Error.Message
		}

		uptime := math.Floor(time.Since(startTime).Seconds())

		resp := map[string]any{
			"ok":       dbOk,
			"db":       dbBody,
			"daemon":   daemonBody,
			"uptime_s": int(uptime),
			"runtime":  runtime.Version(),
		}

		w.Header().Set("Content-Type", "application/json")
		// Node health always returns 200 regardless of db/daemon status.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
