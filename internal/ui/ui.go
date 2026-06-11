// Package ui provides the templ+HTMX general UI module for engram-explorer.
// It mounts at the root of the shared API ServeMux, alongside /doctor/* and /api/*.
// Deps uses only concrete types to avoid import cycles with internal/httpapi.
package ui

import (
	"io/fs"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
	"github.com/a-h/templ"
)

// Deps carries the concrete dependencies required by the ui module.
// It mirrors doctor.Deps intentionally — no Container, no import cycle.
type Deps struct {
	RoDB   sqlite.Querier       // read-only pool; may be nil in tests
	RWDB   sqlite.Querier       // read-write pool; nil in read-only mode
	Config config.Config        // full runtime configuration (immutable snapshot)
	Paths  *config.RuntimePaths // live mutable paths; defaulted from Config in Mount

	// Path-edit callbacks, wired from the HTTP container in production and nil in
	// tests. ReloadEngramDB hot-swaps the SQLite pools; SetClaudeDir updates the
	// transcript directory. Both validate and persist the override.
	ReloadEngramDB func(path string) error
	SetClaudeDir   func(dir string) error

	// NavGroups, when non-nil, is called per-request to obtain the current
	// sidebar navigation groups from the provider registry. When nil the sidebar
	// falls back to the legacy hardcoded two-section layout (engram + claude).
	// Wired in production from Registry.NavGroups via NewContainerWithRegistry.
	NavGroups func() []providers.NavGroup
}

// IsHTMX reports whether the request was issued by HTMX.
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// renderDeps writes a templ component to the response with text/html content
// type. It enriches the context with nav prefs AND, when d.NavGroups is set,
// the current NavGroups from the registry for the data-driven sidebar.
func renderDeps(w http.ResponseWriter, r *http.Request, d Deps, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = c.Render(NavContextWithGroups(r, d.NavGroups), w)
}

// render writes a templ component to the response with text/html content type.
// The context carries the sidebar section-visibility prefs so the shared
// Sidebar can honour them.
// Deprecated: use renderDeps when a Deps is available (passes NavGroups).
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = c.Render(NavContext(r), w)
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
	// Default the live path holder from the immutable Config so tests that build
	// Deps without Paths keep working (handlers read mutable paths via d.Paths).
	if d.Paths == nil {
		d.Paths = config.NewRuntimePaths(d.Config.EngramDbPath, d.Config.ClaudeProjectsDir)
	}

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

	// Claude Code overview route (usage charts only — no list, no filter).
	mux.HandleFunc("GET /cc-overview", handleCCOverviewPage(d))

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
	mux.HandleFunc("POST /settings/engram-db", handleEngramDBPost(d))
	mux.HandleFunc("POST /settings/claude-dir", handleClaudeDirPost(d))
	mux.HandleFunc("POST /settings/nav-visibility", handleNavVisibilityPost())
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
