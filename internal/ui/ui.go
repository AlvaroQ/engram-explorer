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

	// AdvancedView, when non-nil, returns whether the active profile has the
	// advanced-view preference enabled. When nil, advanced view defaults to false.
	AdvancedView func() bool

	// SetAdvancedView persists the advanced-view preference to the active
	// profile in config.json. When nil, the toggle endpoint returns 503.
	SetAdvancedView func(enabled bool) error

	// Live, when non-nil, reports the data handles of the modules serving one
	// request; they stay open until that request's handler returns. Mount
	// calls it on every request and hands the
	// handlers a copy of Deps carrying them, so enabling or disabling a module
	// needs no remount and no handler keeps a closed pool. Memory pages
	// answer 503 (RenderModuleDisabled) while RoDB is nil, Claude Code pages
	// while Claude is false. When nil (tests, legacy path) RoDB/RWDB are used
	// as mounted and no page is gated.
	Live func(r *http.Request) LiveHandles
}

// LiveHandles is the per-request module state returned by Deps.Live.
type LiveHandles struct {
	RoDB   sqlite.Querier // read-only pool; nil when the Engram module is not enabled
	RWDB   sqlite.Querier // read-write pool; nil when writes are not available
	Claude bool           // the Claude Code sessions module is enabled
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
	// Cause is the provider error. It is never rendered as-is: the settings
	// handler turns it into errMsg via ErrorMessage (usererr messages are
	// shown, anything else is logged and replaced by the generic message).
	Cause error
	// errMsg is the display message for the row. Unexported so callers outside
	// the package cannot put raw error text on the page; only Cause feeds it.
	errMsg string
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
// for the data-driven sidebar and Profiles for the account switcher.
func renderDeps(w http.ResponseWriter, r *http.Request, d Deps, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := NavContextWithGroupsAndProfiles(r, d.NavGroups, d.Profiles)
	_ = c.Render(ctx, w)
}

// Render writes a templ component to the response with text/html content type.
// The context carries the sidebar section-visibility prefs so the shared
// Sidebar can honour them. Shared by the doctor package; inside ui prefer
// renderDeps when a Deps is available (it also passes NavGroups).
func Render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = c.Render(NavContext(r), w)
}

// RequireRW wraps a write handler with a read-only guard shared by the ui and
// doctor packages. It blocks the write when no read-write pool is available
// (read-only mode), answering with an ErrorPartial (status 200 so HTMX swaps it).
func RequireRW(writable bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !writable {
			Render(w, r, ErrorPartial("Write operations are not available in read-only mode."))
			return
		}
		h(w, r)
	}
}

// RenderModuleDisabled answers 503 with a localized HTML message for a page
// whose module is disabled: inside the full Layout on a normal navigation
// (sidebar and account switcher from d's NavGroups and Profiles), as a bare
// fragment for htmx requests. Shared by the HTTP layer
// for provider-owned routes.
func RenderModuleDisabled(w http.ResponseWriter, r *http.Request, d Deps) {
	lang := langForRequest(r)
	var c templ.Component = ErrorPartial(T(lang, "common.moduleDisabled"))
	if !IsHTMX(r) {
		c = Layout(T(lang, "common.moduleDisabledTitle"), "", lang,
			themeForRequest(r), sidebarStateForRequest(r), c)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	renderDeps(w, r, d, c)
}

// handlerFactory builds a page handler from Deps.
type handlerFactory func(Deps) http.HandlerFunc

// liveHandler builds the handler per request from Deps carrying the handles
// d.Live reports right now; available, when non-nil, gates the page (503 via
// RenderModuleDisabled when it returns false). Without d.Live the handler is
// built once from d and never gated.
func liveHandler(d Deps, available func(LiveHandles) bool, factory handlerFactory) http.HandlerFunc {
	if d.Live == nil {
		return factory(d)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		h := d.Live(r)
		if available != nil && !available(h) {
			RenderModuleDisabled(w, r, d)
			return
		}
		dd := d
		dd.RoDB, dd.RWDB = h.RoDB, h.RWDB
		factory(dd)(w, r)
	}
}

func engramEnabled(h LiveHandles) bool { return h.RoDB != nil }
func claudeEnabled(h LiveHandles) bool { return h.Claude }

// requireRW applies RequireRW with the read-write pool of d.
func requireRW(d Deps, h http.HandlerFunc) http.HandlerFunc {
	return RequireRW(d.RWDB != nil, h)
}

// Mount registers all UI routes at the root of mux.
func Mount(mux *http.ServeMux, d Deps) {
	// Default the live path holder from the immutable Config so tests that build
	// Deps without Paths keep working (handlers read mutable paths via d.Paths).
	if d.Paths == nil {
		d.Paths = config.NewRuntimePaths(d.Config.EngramDbPath, d.Config.ClaudeProjectsDir)
	}
	// shell pages work with any module state; memory and claude pages answer
	// 503 while their module is disabled (see Deps.Live).
	shell := func(f handlerFactory) http.HandlerFunc { return liveHandler(d, nil, f) }
	memory := func(f handlerFactory) http.HandlerFunc { return liveHandler(d, engramEnabled, f) }
	claude := func(f handlerFactory) http.HandlerFunc { return liveHandler(d, claudeEnabled, f) }

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
	mux.HandleFunc("GET /{$}", shell(handleOverviewPage))

	// Topics routes.
	// GET /topics redirects to the Threads tab on the Memory page (PR6a).
	// GET /topics/list (HTMX partial) is kept intact — unaffected by the redirect.
	// Note: Go 1.22 ServeMux exact-path patterns ensure /topics/list wins over /topics.
	mux.HandleFunc("GET /topics", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/observations?view=threads", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /topics/list", memory(handleTopicsListPartial))

	// Sessions list routes (must be registered before {id} wildcard).
	mux.HandleFunc("GET /sessions", memory(handleSessionsListPage))
	mux.HandleFunc("GET /sessions/list", memory(handleSessionsListPartial))

	// Claude Code route — Overview tab (charts) and Sessions tab (?tab=sessions).
	mux.HandleFunc("GET /cc-overview", claude(handleCCOverviewPage))

	// GET /cc-sessions redirects to the Sessions tab on the CC page (PR8).
	// /cc-sessions/list (HTMX load-more partial) and /cc-sessions/{project}/{id}
	// (detail) are kept intact — Go 1.22 exact-path matching ensures they win
	// over the shorter /cc-sessions pattern.
	mux.HandleFunc("GET /cc-sessions", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/cc-overview?tab=sessions", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /cc-sessions/list", claude(handleCCSessionsListPartial))
	mux.HandleFunc("GET /cc-sessions/{project}/{id}", claude(handleCCSessionDetailPage))

	// Session detail route.
	mux.HandleFunc("GET /sessions/{id}", memory(handleSessionDetailPage))

	// Prompts routes.
	// GET /prompts redirects to the Conversations tab on the Memory page (PR6a).
	// GET /prompts/list (HTMX partial) is kept intact — unaffected by the redirect.
	// Note: Go 1.22 ServeMux exact-path patterns ensure /prompts/list wins over /prompts.
	mux.HandleFunc("GET /prompts", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/observations?view=conversations", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /prompts/list", memory(handlePromptsListPartial))

	// Projects routes.
	// Note: Go 1.22 mux resolves exact paths over wildcards, so /projects and
	// /projects/list are matched before /projects/{project}.
	mux.HandleFunc("GET /projects", memory(handleProjectsPage))
	mux.HandleFunc("GET /projects/list", memory(handleProjectsListPartial))
	mux.HandleFunc("GET /projects/{project}", memory(handleProjectDetailPage))
	mux.HandleFunc("POST /projects/{project}/merge", memory(handleProjectMergePost))

	// Settings routes.
	mux.HandleFunc("GET /settings", shell(handleSettingsPage))
	mux.HandleFunc("POST /settings/engram-db", shell(handleEngramDBPost))
	mux.HandleFunc("POST /settings/claude-dir", shell(handleClaudeDirPost))
	mux.HandleFunc("POST /settings/nav-visibility", handleNavVisibilityPost())
	mux.HandleFunc("POST /theme", handleThemePost())
	mux.HandleFunc("POST /lang", handleLangPost())

	// Settings → Advanced view toggle (PR6b).
	mux.HandleFunc("POST /settings/advanced-view", shell(handleAdvancedViewPost))

	// Settings → Modules routes (WU-8).
	mux.HandleFunc("POST /settings/modules/{id}/toggle", shell(handleModulesTogglePost))
	mux.HandleFunc("POST /settings/modules/{id}/path", shell(handleModulesPathPost))

	// Settings → Accounts routes (WU-9).
	mux.HandleFunc("GET /settings/accounts", shell(handleAccountsPage))
	mux.HandleFunc("POST /settings/profile/switch", shell(handleProfileSwitchPost))
	mux.HandleFunc("POST /settings/profile/create", shell(handleProfileCreatePost))
	mux.HandleFunc("POST /settings/profile/delete", shell(handleProfileDeletePost))

	// Settings → CC Accounts routes (Slice 2b).
	mux.HandleFunc("POST /settings/cc-accounts", shell(handleCCAccountAddPost))
	mux.HandleFunc("POST /settings/cc-accounts/{id}/remove", shell(handleCCAccountRemovePost))
	mux.HandleFunc("POST /settings/cc-accounts/{id}/toggle", shell(handleCCAccountTogglePost))

	// Observations routes.
	// /list and /{id} must be registered before the wildcard so Go 1.22 exact
	// matching takes precedence.
	mux.HandleFunc("GET /observations", memory(handleObservationsPage))
	mux.HandleFunc("GET /observations/list", memory(handleObservationsListPartial))
	mux.HandleFunc("GET /observations/{id}", memory(handleObservationDetailPartial))

	// Brain page (React island — Three.js graph).
	mux.HandleFunc("GET /brain", memory(handleBrainPage))

	// Sidebar status pill partial (polled by HTMX from the sidebar footer).
	mux.HandleFunc("GET /partials/status-pill", shell(handleStatusPillPartial))
}
