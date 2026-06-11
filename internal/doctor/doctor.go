// Package doctor provides the templ+HTMX diagnostics module for engram-explorer.
// It mounts at /doctor/* on the shared API ServeMux, completely independent of
// the React SPA.  Deps uses only concrete types to avoid import cycles with
// internal/httpapi.
package doctor

import (
	"io/fs"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
	"github.com/a-h/templ"
)

// Deps carries the concrete dependencies required by the doctor module.
// It intentionally uses sqlite.Querier and config.Config directly — never the
// httpapi Container — so that internal/doctor can be imported by internal/httpapi
// without creating an import cycle.
type Deps struct {
	RoDB   sqlite.Querier       // read-only pool; may be nil in tests
	RWDB   sqlite.Querier       // read-write pool; nil when read-only mode is active
	Config config.Config        // full runtime configuration (immutable snapshot)
	Paths  *config.RuntimePaths // live mutable paths; defaulted from Config in Mount

	// Cloud is an optional CloudController override.  When non-nil it is used
	// by the sync write handlers instead of constructing a concrete
	// *services.CloudControlService inside Mount.  Production callers leave this
	// nil (Mount builds the concrete service); tests inject a fake to exercise
	// success paths without spawning a real CLI subprocess.
	Cloud CloudController

	// DaemonPing is an optional override for the daemon availability check used
	// by handleSyncIssues.  When non-nil it is called instead of making a real
	// HTTP round-trip to the daemon.  Production callers leave this nil; tests
	// inject func() bool { return true } to simulate an available daemon.
	DaemonPing func() bool
}

// IsHTMX reports whether the request was issued by HTMX (i.e. the HX-Request
// header is present and set to "true").
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// render writes a templ component to the response.  It always sets the
// Content-Type header to text/html; charset=utf-8.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Carry the sidebar section-visibility prefs (shared ui.Sidebar reads them).
	_ = c.Render(ui.NavContext(r), w)
}

// requireRW wraps a write handler with a read-only guard. It blocks the write
// in demo mode (the bundled sample DB must stay pristine) and when RWDB is nil
// (read-only mode), so every write route shares a single source of truth for
// the writability check.
func requireRW(d Deps, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Config.DemoMode {
			render(w, r, ErrorPartial("Write operations are disabled in demo mode."))
			return
		}
		if d.RWDB == nil {
			render(w, r, ErrorPartial("Write operations are not available in read-only mode."))
			return
		}
		h(w, r)
	}
}

// Mount registers all /doctor/* routes on mux.
// Call this inside httpapi.NewServeMux after the cloud routes.
func Mount(mux *http.ServeMux, d Deps) {
	// Default the live path holder from the immutable Config so tests that build
	// Deps without Paths keep working.
	if d.Paths == nil {
		d.Paths = config.NewRuntimePaths(d.Config.EngramDbPath, d.Config.ClaudeProjectsDir)
	}

	// Static assets (/doctor/static/*).
	staticSub, err := fs.Sub(StaticFS, "static")
	if err != nil {
		// embed.FS always has the "static" directory at compile time.
		panic("doctor: could not sub static FS: " + err.Error())
	}
	fileServer := http.FileServerFS(staticSub)
	mux.Handle("GET /doctor/static/",
		http.StripPrefix("/doctor/static/", fileServer),
	)

	// Consolidated maintenance page — reachable from Settings.
	// Registered before /doctor/* redirects so Go's ServeMux matches the literal
	// path first (exact wins over prefix).
	mux.HandleFunc("GET /settings/maintenance", handleMaintenancePage(d))

	// Root: redirect to consolidated maintenance page.
	mux.HandleFunc("GET /doctor/", func(w http.ResponseWriter, r *http.Request) {
		// Only match the exact root; sub-paths are handled by their own entries.
		if r.URL.Path != "/doctor/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/settings/maintenance", http.StatusMovedPermanently)
	})

	// -----------------------------------------------------------------------
	// Orphans routes (Slice 1)
	// -----------------------------------------------------------------------

	// GET /doctor/orphans — 301 redirect to the consolidated maintenance page.
	// HTMX partials (/doctor/orphans/list, etc.) are NOT redirected.
	mux.HandleFunc("GET /doctor/orphans", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/settings/maintenance#unassigned", http.StatusMovedPermanently)
	})

	// GET /doctor/orphans/list — bare list partial; hx-get deferred load target.
	mux.HandleFunc("GET /doctor/orphans/list", handleOrphansListPartial(d))

	// GET /doctor/orphans/observations/{id}/detail — observation detail dialog.
	// Renders a modal with metadata, session working directory, full content,
	// and the assign/delete actions, so the user can assign from inside it.
	mux.HandleFunc("GET /doctor/orphans/observations/{id}/detail", handleOrphanObservationDetail(d))

	// POST /doctor/orphans/{entity}/{id}/project — assign entity to a project.
	mux.HandleFunc("POST /doctor/orphans/{entity}/{id}/project", requireRW(d, handleOrphansAssign(d)))

	// DELETE /doctor/orphans/{entity}/{id} — delete orphaned entity.
	mux.HandleFunc("DELETE /doctor/orphans/{entity}/{id}", requireRW(d, handleOrphansDelete(d)))

	// -----------------------------------------------------------------------
	// Sync routes (Slice 2)
	// -----------------------------------------------------------------------

	// Resolve the CloudController: use the injected fake when available
	// (test path), otherwise construct the real *services.CloudControlService
	// (production path).  This keeps the production call site in
	// internal/httpapi/server.go unchanged — it never sets Deps.Cloud.
	var cloud CloudController
	if d.Cloud != nil {
		cloud = d.Cloud
	} else {
		cloud = services.NewCloudControlService(services.CloudControlOptions{
			AuditLogPath:  d.Config.AuditLogPath,
			EngramDataDir: d.Paths.DataDir(),
			RWDB:          d.RWDB,
		})
	}

	// GET /doctor/sync — 301 redirect to the consolidated maintenance page.
	// HTMX partials (/doctor/sync/projects, etc.) are NOT redirected.
	mux.HandleFunc("GET /doctor/sync", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/settings/maintenance#cloud", http.StatusMovedPermanently)
	})

	// GET /doctor/sync/projects — projects table partial (polled every 15s).
	// Must be registered before /doctor/sync/{project} so the literal path wins.
	// Takes cloud so it can probe capabilities and gate the action buttons.
	mux.HandleFunc("GET /doctor/sync/projects", handleSyncProjectsList(d, cloud))

	// GET /doctor/sync/issues — issues banner partial (polled every 15s).
	// Must be registered before /doctor/sync/{project} (same reason).
	mux.HandleFunc("GET /doctor/sync/issues", handleSyncIssues(d))

	// GET /doctor/sync/{project} — project detail partial.
	// Go 1.22+ ServeMux: literal paths above win over wildcards automatically,
	// but we register them first for clarity.
	mux.HandleFunc("GET /doctor/sync/{project}", handleSyncProjectDetail(d))

	// POST /doctor/sync/{project}/enroll — enroll project in cloud sync.
	mux.HandleFunc("POST /doctor/sync/{project}/enroll",
		requireRW(d, handleSyncCloudAction(d, cloud, "Enroll", cloud.Enroll)))

	// POST /doctor/sync/{project}/unenroll — unenroll project from cloud sync.
	mux.HandleFunc("POST /doctor/sync/{project}/unenroll",
		requireRW(d, handleSyncCloudAction(d, cloud, "Unenroll", cloud.Unenroll)))

	// POST /doctor/sync/{project}/sync — trigger manual sync for a project.
	mux.HandleFunc("POST /doctor/sync/{project}/sync", requireRW(d, handleSyncTrigger(d, cloud)))

	// POST /doctor/sync/{project}/delete — permanently delete an unenrolled,
	// observation-free project (removes its sessions and prompts).
	mux.HandleFunc("POST /doctor/sync/{project}/delete", requireRW(d, handleSyncDeleteProject(d, cloud)))
}
