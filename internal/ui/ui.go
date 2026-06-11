// Package ui provides the templ+HTMX general UI module for engram-explorer.
// It mounts at the root of the shared API ServeMux, alongside /doctor/* and /api/*.
// Deps uses only concrete types to avoid import cycles with internal/httpapi.
package ui

import (
	"context"
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
	RoDB     sqlite.Querier       // read-only pool; may be nil in tests
	RWDB     sqlite.Querier       // read-write pool; nil in read-only mode
	Config   config.Config        // full runtime configuration (immutable snapshot)
	Paths    *config.RuntimePaths // live mutable paths; defaulted from Config in Mount
	DemoMode bool                 // when true, show demo banner and block write endpoints

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

	// Modules, when non-nil, returns a snapshot of all registered providers for
	// the Settings → Modules page. When nil the Modules section is hidden.
	// Wired in production from Registry.AllProviderMetas via mountRegistryRoutes.
	Modules func() []ModuleInfo

	// ToggleModule enables or disables a provider by ID. When nil the toggle
	// endpoint returns 503. On success the caller should persist the change to
	// the active profile in config.json.
	ToggleModule func(id string, enabled bool) error

	// ValidateModulePath runs provider-specific path validation (Tier-1 manual
	// path entry). On success the provider is opened and the path is persisted.
	// When nil the path-entry endpoint returns 503.
	ValidateModulePath func(ctx context.Context, id, path string) error

	// Profiles, when non-nil, returns the list of named profiles for the
	// Settings → Accounts page and the account switcher in the sidebar.
	Profiles func() []ProfileInfo

	// ActiveProfile, when non-nil, returns the name of the currently active profile.
	ActiveProfile func() string

	// SwitchProfile switches the registry and persistent config to the named profile.
	// Returns an error if any provider fails to open in the target profile — the old
	// profile remains fully active on failure (abort-on-error per design verdict 2).
	// When nil the switch endpoint returns 503.
	SwitchProfile func(name string) error

	// CreateProfile creates a new named profile (empty, or cloned from active).
	// Returns an error if a profile with that name already exists.
	// When nil the create endpoint returns 503.
	CreateProfile func(name string) error

	// DeleteProfile deletes the named profile from the persistent store.
	// Returns an error if the profile is active or if it is the last remaining one.
	// When nil the delete endpoint returns 503.
	DeleteProfile func(name string) error

	// ActiveCount, when non-nil, returns the number of providers currently in
	// Enabled state. Used by handleOverviewPage to decide whether to show the
	// onboarding zero-state or the regular overview. When nil (legacy/no-registry
	// path), the overview is always shown.
	ActiveCount func() int

	// CCAccountSources, when non-nil, returns the list of enabled CC account
	// sources (each source wraps a CCProjectsReader for one ~/.claude directory).
	// When nil the CC handlers fall back to a single DiskProjectsReader using
	// d.Paths.ClaudeDir(), preserving backward compatibility with legacy/test paths.
	CCAccountSources func() []services.CCAccountSource

	// CCAccounts, when non-nil, returns all CC accounts (enabled and disabled)
	// as CCAccountInfo view-models for the Settings → CC Accounts section.
	CCAccounts func() []CCAccountInfo

	// AddCCAccount validates the given path (via the cc-sessions provider) and
	// then creates a new CC account in the profile store.
	AddCCAccount func(label, path string) error

	// UpdateCCAccount updates an existing CC account's label, path and enabled
	// state. Used by the toggle and edit flows.
	UpdateCCAccount func(id, label, path string, enabled bool) error

	// RemoveCCAccount removes the CC account with the given id from the store.
	// Returns an error if it is the last remaining account.
	RemoveCCAccount func(id string) error
}

// ---------------------------------------------------------------------------
// CC Accounts view-models (Settings → CC Accounts section)
// ---------------------------------------------------------------------------

// CCAccountInfo is a view-model for one CC account row in the Settings page.
type CCAccountInfo struct {
	ID      string
	Label   string
	Path    string
	Enabled bool
}

// ---------------------------------------------------------------------------
// Modules view-models (Settings → Modules page)
// ---------------------------------------------------------------------------

// ModuleState is the display state of a provider in the Settings Modules page.
type ModuleState int

const (
	// ModuleEnabled — provider is open and serving requests.
	ModuleEnabled ModuleState = iota
	// ModuleDisabled — provider is user-toggled off.
	ModuleDisabled
	// ModuleDetected — source is visible on disk but not opened (Tier-1 path entry).
	ModuleDetected
	// ModuleErrored — provider failed to open.
	ModuleErrored
	// ModuleRegistered — provider is known but not yet detected.
	ModuleRegistered
)

// ModuleTier classifies whether a provider supports manual path entry.
type ModuleTier int

const (
	// ModuleTier1 providers offer manual path entry when not auto-detected.
	ModuleTier1 ModuleTier = iota
	// ModuleTier2 providers are hidden when not auto-detected.
	ModuleTier2
)

// ModuleInfo is a view-model for one provider row in the Settings Modules page.
type ModuleInfo struct {
	ID          string
	DisplayName string
	Tier        ModuleTier
	State       ModuleState
	Path        string // current path from config; may be empty
	Err         string // error message when State == ModuleErrored
}

// ---------------------------------------------------------------------------
// Accounts view-models (Settings → Accounts page + account switcher)
// ---------------------------------------------------------------------------

// ProfileInfo is a view-model for one profile row in the Settings → Accounts
// page and the sidebar account switcher.
type ProfileInfo struct {
	Name   string
	Active bool
}

// IsHTMX reports whether the request was issued by HTMX.
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// renderDeps writes a templ component to the response with text/html content
// type. It enriches the context with nav prefs, NavGroups from the registry
// for the data-driven sidebar, Profiles for the account switcher, and the
// demo-mode flag for the demo banner.
func renderDeps(w http.ResponseWriter, r *http.Request, d Deps, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := NavContextWithGroupsAndProfiles(r, d.NavGroups, d.Profiles)
	if d.DemoMode {
		ctx = WithDemoMode(ctx, true)
	}
	_ = c.Render(ctx, w)
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
		if d.DemoMode {
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

	// Settings → Modules routes (WU-8).
	mux.HandleFunc("POST /settings/modules/{id}/toggle", handleModulesTogglePost(d))
	mux.HandleFunc("POST /settings/modules/{id}/path", handleModulesPathPost(d))

	// Settings → Accounts routes (WU-9).
	mux.HandleFunc("GET /settings/accounts", handleAccountsPage(d))
	mux.HandleFunc("POST /settings/profile/switch", handleProfileSwitchPost(d))
	mux.HandleFunc("POST /settings/profile/create", handleProfileCreatePost(d))
	mux.HandleFunc("POST /settings/profile/delete", handleProfileDeletePost(d))

	// Settings → CC Accounts routes (Slice 2b).
	mux.HandleFunc("POST /settings/cc-accounts", handleCCAccountAddPost(d))
	mux.HandleFunc("POST /settings/cc-accounts/{id}/remove", handleCCAccountRemovePost(d))
	mux.HandleFunc("POST /settings/cc-accounts/{id}/toggle", handleCCAccountTogglePost(d))

	// Observations routes.
	// /list and /{id} must be registered before the wildcard so Go 1.22 exact
	// matching takes precedence.
	mux.HandleFunc("GET /observations", handleObservationsPage(d))
	mux.HandleFunc("GET /observations/list", handleObservationsListPartial(d))
	mux.HandleFunc("GET /observations/{id}", handleObservationDetailPartial(d))

	// Brain page (React island — Three.js graph).
	mux.HandleFunc("GET /brain", handleBrainPage(d))

	// Sidebar status pill partial (polled by HTMX from the sidebar footer).
	mux.HandleFunc("GET /partials/status-pill", handleStatusPillPartial(d))

	// Sidebar "Sync cloud" action — triggers a cloud sync for all enrolled
	// projects and renders the result fragment back into the sidebar.
	mux.HandleFunc("POST /partials/sync-cloud", handleSyncCloudPost(d, cloud))
}
