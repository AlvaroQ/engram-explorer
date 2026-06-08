// Package ui provides the templ+HTMX general UI module for engram-explorer.
// It mounts at the root of the shared API ServeMux, alongside /doctor/* and /api/*.
// Deps uses only concrete types to avoid import cycles with internal/httpapi.
package ui

import (
	"database/sql"
	"io/fs"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/a-h/templ"
)

// Deps carries the concrete dependencies required by the ui module.
// It mirrors doctor.Deps intentionally — no Container, no import cycle.
type Deps struct {
	RoDB   *sql.DB       // read-only pool; may be nil in tests
	RWDB   *sql.DB       // read-write pool; nil in read-only mode
	Config config.Config // full runtime configuration
}

// IsHTMX reports whether the request was issued by HTMX.
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// render writes a templ component to the response with text/html content type.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = c.Render(r.Context(), w)
}

// requireRW wraps a write handler with a read-only guard.
func requireRW(d Deps, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.RWDB == nil {
			render(w, r, ErrorPartial("Write operations are not available in read-only mode."))
			return
		}
		h(w, r)
	}
}

// Mount registers all UI routes at the root of mux. It constructs the production
// CloudControlService for the projects enroll/unenroll endpoints.
func Mount(mux *http.ServeMux, d Deps) {
	cloud := services.NewCloudControlService(services.CloudControlOptions{
		AuditLogPath:  d.Config.AuditLogPath,
		EngramDataDir: d.Config.EngramDataDir,
		RWDB:          d.RWDB,
	})
	MountWithCloud(mux, d, cloud)
}

// MountWithCloud registers all UI routes at the root of mux with an explicit
// cloud controller. This overload is used by tests to inject a fake implementation
// without spawning a real CLI subprocess.
func MountWithCloud(mux *http.ServeMux, d Deps, cloud projectsCloud) {
	// Static assets (/static/*).
	staticSub, err := fs.Sub(StaticFS, "static")
	if err != nil {
		panic("ui: could not sub static FS: " + err.Error())
	}
	fileServer := http.FileServerFS(staticSub)
	mux.Handle("GET /static/",
		http.StripPrefix("/static/", fileServer),
	)

	// Root: overview page (home).
	// GET /{$} is Go 1.22 exact-match for "/"; it does not catch-all like GET /.
	mux.HandleFunc("GET /{$}", handleOverviewPage(d))

	// Topics routes.
	mux.HandleFunc("GET /topics", handleTopicsPage(d))
	mux.HandleFunc("GET /topics/list", handleTopicsListPartial(d))

	// Sessions list routes (must be registered before {id} wildcard).
	mux.HandleFunc("GET /sessions", handleSessionsListPage(d))
	mux.HandleFunc("GET /sessions/list", handleSessionsListPartial(d))

	// Claude Code sessions routes (live .jsonl reader — no SQLite).
	// /list and /{project}/{id} must be registered before the wildcard so Go 1.22
	// exact matching takes precedence over the two-segment wildcard.
	mux.HandleFunc("GET /cc-sessions", handleCCSessionsListPage(d))
	mux.HandleFunc("GET /cc-sessions/list", handleCCSessionsListPartial(d))
	mux.HandleFunc("GET /cc-sessions/{project}/{id}", handleCCSessionDetailPage(d))

	// Session detail route.
	mux.HandleFunc("GET /sessions/{id}", handleSessionDetailPage(d))

	// Prompts routes.
	mux.HandleFunc("GET /prompts", handlePromptsPage(d))
	mux.HandleFunc("GET /prompts/list", handlePromptsListPartial(d))

	// Projects routes.
	// Note: Go 1.22 mux resolves exact paths over wildcards, so /projects and
	// /projects/list are matched before /projects/{project}.
	mux.HandleFunc("GET /projects", handleProjectsPage(d, cloud))
	mux.HandleFunc("GET /projects/list", handleProjectsListPartial(d, cloud))
	mux.HandleFunc("GET /projects/{project}", handleProjectDetailPage(d))
	mux.HandleFunc("POST /projects/{project}/enroll", requireRW(d, handleProjectsEnroll(d, cloud)))
	mux.HandleFunc("POST /projects/{project}/unenroll", requireRW(d, handleProjectsUnenroll(d, cloud)))

	// Settings routes.
	mux.HandleFunc("GET /settings", handleSettingsPage(d))
	mux.HandleFunc("POST /theme", handleThemePost())
	mux.HandleFunc("POST /lang", handleLangPost())

	// Observations routes.
	// /list and /{id} must be registered before the wildcard so Go 1.22 exact
	// matching takes precedence.
	mux.HandleFunc("GET /observations", handleObservationsPage(d))
	mux.HandleFunc("GET /observations/list", handleObservationsListPartial(d))
	mux.HandleFunc("GET /observations/{id}", handleObservationDetailPartial(d))

	// Brain page (React island — Three.js graph).
	mux.HandleFunc("GET /brain", handleBrainPage(d))
}
