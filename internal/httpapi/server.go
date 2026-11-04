package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/daemon"
	"github.com/AlvaroQ/engram-explorer/internal/doctor"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// startTime is recorded once so uptime_s can be computed in the health handler.
var startTime = time.Now()

// NewServeMux builds the ServeMux with all registered routes and middleware chain.
// Middleware order (preserved from the original Hono backend during migration):
//  1. Secure headers
//  2. Request logging
//  3. CORS (dev only, allow-list)
//  4. Route dispatch (rate limiters applied per-route in mux registration)
//
// When c.Registry is non-nil (registry-backed container from WU-5+), routes
// are mounted by looping over enabled providers. Provider-specific routes
// (Engram SQLite, CC sessions) are wired only when the corresponding provider
// is active; if no providers are active, only the health endpoint and the UI
// shell are registered — the server starts successfully with zero providers.
//
// When c.Registry is nil (legacy direct-mount mode, used by existing tests),
// the original direct-mount code path is used unchanged.
func NewServeMux(c *Container) http.Handler {
	// Apply middleware stack (outermost = first to run).
	var handler http.Handler = newRouter(c)
	handler = corsMiddleware(handler, trustedDevOrigins(c.Config.Env))
	handler = requestLogger(c.Logger, handler)
	handler = secureHeaders(handler)

	return handler
}

// newRouter builds the ServeMux and mounts every route (no middleware).
func newRouter(c *Container) *http.ServeMux {
	mux := http.NewServeMux()

	// --- Phase 1 routes ---
	mux.HandleFunc("GET /api/health", healthHandler(c))

	if c.Registry != nil {
		// Registry-backed path (WU-5+): mount per-provider routes by looping
		// over enabled instances. Each provider's Routes() stub is a no-op;
		// the real wiring happens here by type-asserting the instance to access
		// provider-specific deps (Engram Deps/DoctorDeps, CC RuntimePaths).
		mountRegistryRoutes(mux, c)
	} else {
		// Legacy direct-mount path: preserved verbatim so all existing tests
		// (which call NewContainer and pass a nil Registry) keep passing.
		mountLegacyRoutes(mux, c)
	}

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

	return mux
}

// mountLegacyRoutes registers all routes in the original direct-mount style.
// Used when c.Registry is nil (existing tests, backward-compat path).
func mountLegacyRoutes(mux *http.ServeMux, c *Container) {
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

	// Cloud is read-only: no enroll/unenroll/sync routes. Sync diagnostics
	// (syncRoutes) and the GET daemon proxy (daemonRoutes) cover it.

	// --- Doctor diagnostics module (templ+HTMX) ---
	doctor.Mount(mux, doctor.Deps{
		RoDB:   c.RoDB,
		RWDB:   c.RWDB,
		Config: c.Config,
		Paths:  c.Paths,
	})

	// --- UI module — server-rendered pages (templ+HTMX) ---
	ui.Mount(mux, ui.Deps{
		RoDB:           c.RoDB,
		RWDB:           c.RWDB,
		Config:         c.Config,
		Paths:          c.Paths,
		DemoMode:       c.Config.DemoMode,
		ReloadEngramDB: c.ReloadEngramDB,
		SetClaudeDir:   c.SetClaudeDir,
	})
}

// mountRegistryRoutes registers routes for all enabled providers in the
// registry. Engram-specific routes (observations, prompts, sessions, write,
// sync, doctor, etc.) are mounted only when the Engram provider is active.
// CC-sessions routes are mounted only when the CC provider is active.
// The UI shell (GET /, settings, etc.) and daemon routes are always registered.
func mountRegistryRoutes(mux *http.ServeMux, c *Container) {
	reg := c.Registry
	engramMounted := false

	// Every profile callback below goes through c.ProfileStore, which owns the
	// active profile name and guards all access with a lock. Nothing captures
	// the active profile by value, so after a switch every read and write
	// targets the new active profile.
	store := c.ProfileStore

	for _, entry := range reg.Entries() {
		if entry.State != providers.Enabled {
			continue
		}
		inst := reg.Instance(entry.ID)
		if inst == nil {
			continue
		}

		switch entry.ID {
		case "engram":
			// Mount all Engram-specific routes via the package-level accessor.
			if deps, ddeps, ok := engramprovider.EngramDeps(inst); ok {
				// Propagate live handles into container fields so that route-group
				// functions (which accept *Container) pick up the open DB pools.
				c.RoDB = deps.RoDB
				c.RWDB = deps.RWDB
				c.Paths = deps.Paths

				observationsRoutes(mux, c)
				promptsRoutes(mux, c)
				projectsRoutes(mux, c)
				overviewRoutes(mux, c)
				orphansRoutes(mux, c)
				sessionsRoutes(mux, c)
				syncRoutes(mux, c)
				graphRoutes(mux, c)
				writeRoutes(mux, c)

				doctor.Mount(mux, ddeps)

				// Wire the container's ReloadEngramDB / SetClaudeDir callbacks so
				// the existing settings handlers (POST /settings/engram-db etc.)
				// keep working on the registry-backed path.
				deps.ReloadEngramDB = c.ReloadEngramDB
				deps.SetClaudeDir = c.SetClaudeDir
				// Wire NavGroups so the sidebar is data-driven from the registry.
				deps.NavGroups = reg.NavGroups
				// Wire WU-8 Modules callbacks.
				deps.Modules = buildModulesFn(reg, store)
				deps.ToggleModule = buildToggleModuleFn(reg, store)
				deps.ValidateModulePath = buildValidateModulePathFn(reg, store)
				// Wire WU-9 Accounts callbacks.
				deps.Profiles = buildProfilesFn(store)
				deps.ActiveProfile = buildActiveProfileFn(store)
				deps.SwitchProfile = buildSwitchProfileFn(reg, store)
				deps.CreateProfile = buildCreateProfileFn(store)
				deps.DeleteProfile = buildDeleteProfileFn(store)
				// Wire WU-10 ActiveCount for onboarding zero-state branching.
				deps.ActiveCount = reg.ActiveCount
				// Wire WU-11 CC account sources for multi-account read path.
				deps.CCAccountSources = buildCCAccountSourcesFn(store, c.Paths.ClaudeDir(), c.Config.ConfigHome)
				// Wire Slice 2b CC accounts management callbacks.
				deps.CCAccounts = buildCCAccountsFn(store)
				deps.AddCCAccount = buildAddCCAccountFn(reg, store)
				deps.UpdateCCAccount = buildUpdateCCAccountFn(store)
				deps.RemoveCCAccount = buildRemoveCCAccountFn(store)
				// Wire PR6b advanced-view per-profile preference.
				deps.AdvancedView = buildAdvancedViewFn(store)
				deps.SetAdvancedView = buildSetAdvancedViewFn(store)
				// Thread demo mode into the UI layer.
				deps.DemoMode = c.Config.DemoMode
				ui.Mount(mux, deps)
				engramMounted = true
			}
		case "cc-sessions":
			// Mount CC-sessions routes via the package-level accessor.
			if rtp, ok := ccsessions.CCRuntimePaths(inst); ok {
				c.Paths.SetClaudeDir(rtp.ClaudeDir())
				ccSessionsRoutes(mux, c)
			}
		}
	}

	// Always register daemon routes (independent of provider state).
	daemonRoutes(mux, c)

	// Always mount the UI shell (GET /, settings, etc.) — it renders a working
	// skeleton even with zero providers active (onboarding zero-state is WU-10).
	// When Engram was not mounted above, ui.Mount receives nil RoDB/RWDB so
	// data-fetch handlers degrade gracefully, but the shell itself renders.
	if !engramMounted {
		ui.Mount(mux, ui.Deps{
			RoDB:               c.RoDB,
			RWDB:               c.RWDB,
			Config:             c.Config,
			Paths:              c.Paths,
			DemoMode:           c.Config.DemoMode,
			ReloadEngramDB:     c.ReloadEngramDB,
			SetClaudeDir:       c.SetClaudeDir,
			NavGroups:          reg.NavGroups,
			Modules:            buildModulesFn(reg, store),
			ToggleModule:       buildToggleModuleFn(reg, store),
			ValidateModulePath: buildValidateModulePathFn(reg, store),
			Profiles:           buildProfilesFn(store),
			ActiveProfile:      buildActiveProfileFn(store),
			SwitchProfile:      buildSwitchProfileFn(reg, store),
			CreateProfile:      buildCreateProfileFn(store),
			DeleteProfile:      buildDeleteProfileFn(store),
			// WU-10: wire ActiveCount for onboarding zero-state branching.
			ActiveCount: reg.ActiveCount,
			// WU-11: wire CC account sources for multi-account read path.
			CCAccountSources: buildCCAccountSourcesFn(store, c.Paths.ClaudeDir(), c.Config.ConfigHome),
			// Slice 2b: CC accounts management callbacks.
			CCAccounts:      buildCCAccountsFn(store),
			AddCCAccount:    buildAddCCAccountFn(reg, store),
			UpdateCCAccount: buildUpdateCCAccountFn(store),
			RemoveCCAccount: buildRemoveCCAccountFn(store),
			// PR6b: advanced-view per-profile preference.
			AdvancedView:    buildAdvancedViewFn(store),
			SetAdvancedView: buildSetAdvancedViewFn(store),
		})
	}
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
// When c.Registry is non-nil, the response also includes a "providers" array
// aggregated from all registered providers (per design decision 6). The legacy
// "db" field is preserved as a back-compat alias mirroring the Engram entry.
//
// The daemon ping is a best-effort HTTP call; it never makes the handler fatal.
func healthHandler(c *Container) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		var topOk bool
		var dbBody map[string]any
		resp := map[string]any{
			"daemon":   daemonBody,
			"uptime_s": int(uptime),
			"runtime":  runtime.Version(),
		}

		if c.Registry != nil {
			// Registry-backed path: aggregate health from all registered providers.
			var providerHealths []providers.ProviderHealth
			topOk, providerHealths = c.Registry.AggregateHealth(r.Context())

			// Build the "providers" array for the new shape.
			// Always use a non-nil slice so the JSON output is [] not null.
			provs := make([]map[string]any, 0, len(providerHealths))
			for _, ph := range providerHealths {
				entry := map[string]any{
					"id":     ph.ID,
					"ok":     ph.OK,
					"active": ph.Active,
				}
				if ph.Path != "" {
					entry["path"] = ph.Path
				}
				if ph.Error != "" {
					entry["error"] = ph.Error
				}
				provs = append(provs, entry)
			}
			resp["providers"] = provs

			// Build the legacy "db" alias: mirrors the Engram provider entry if
			// present, otherwise reflects overall ok.
			dbOk := false
			dbPath := c.Paths.EngramDB()
			for _, ph := range providerHealths {
				if ph.ID == "engram" {
					dbOk = ph.OK
					if ph.Path != "" {
						dbPath = ph.Path
					}
					break
				}
			}
			dbBody = map[string]any{
				"ok":   dbOk,
				"path": dbPath,
			}
		} else {
			// Legacy path: direct DB ping (backward compat for existing tests).
			dbOk := false
			var dbErr error
			if c.RoDB != nil {
				var n int
				dbErr = c.RoDB.QueryRow("SELECT 1").Scan(&n)
				dbOk = dbErr == nil && n == 1
			}
			topOk = dbOk
			dbBody = map[string]any{
				"ok":   dbOk,
				"path": c.Paths.EngramDB(),
			}
		}

		resp["ok"] = topOk
		resp["db"] = dbBody

		w.Header().Set("Content-Type", "application/json")
		// Node health always returns 200 regardless of db/daemon status.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// ---------------------------------------------------------------------------
// WU-8: Modules page helpers — registry → ui.ModuleInfo view-model
// ---------------------------------------------------------------------------

// buildModulesFn returns a closure that maps Registry.AllProviderMetas() to
// []ui.ModuleInfo view-models for the Settings → Modules page. The store (may
// be nil) supplies the active profile's path for each provider.
func buildModulesFn(reg *providers.Registry, store *config.Store) func() []ui.ModuleInfo {
	return func() []ui.ModuleInfo {
		var active config.Profile
		if store != nil {
			_, active, _ = store.ActiveProfile()
		}
		metas := reg.AllProviderMetas()
		out := make([]ui.ModuleInfo, 0, len(metas))
		for _, m := range metas {
			out = append(out, ui.ModuleInfo{
				ID:          m.ID,
				DisplayName: m.DisplayName,
				Tier:        registryTierToUITier(m.Tier),
				State:       registryStateToUIState(m.State),
				Path:        active.ProviderCfg(m.ID).Path,
				Cause:       m.Err,
			})
		}
		return out
	}
}

// registryTierToUITier converts a providers.Tier to a ui.ModuleTier.
func registryTierToUITier(t providers.Tier) ui.ModuleTier {
	if t == providers.Tier2 {
		return ui.ModuleTier2
	}
	return ui.ModuleTier1
}

// registryStateToUIState converts a providers.State to a ui.ModuleState.
func registryStateToUIState(s providers.State) ui.ModuleState {
	switch s {
	case providers.Enabled:
		return ui.ModuleEnabled
	case providers.Disabled:
		return ui.ModuleDisabled
	case providers.Detected:
		return ui.ModuleDetected
	case providers.Errored:
		return ui.ModuleErrored
	default:
		return ui.ModuleRegistered
	}
}

// errNoProfileStore is returned by profile callbacks on paths built without a
// store (legacy containers).
var errNoProfileStore = fmt.Errorf("profile store not available")

// buildToggleModuleFn returns a closure that enables or disables a provider in
// the registry and persists the change to the active profile in config.json.
// The registry call and the save run inside one store transaction, so the
// toggle always lands in the profile that is active at that moment.
func buildToggleModuleFn(reg *providers.Registry, store *config.Store) func(id string, enabled bool) error {
	return func(id string, enabled bool) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.UpdateActive(func(_ string, p *config.Profile) error {
			pc := p.ProviderCfg(id)
			if enabled {
				// Open with the effective config (env overrides applied); only
				// the enabled flag is persisted.
				cfg := providerConfigFrom(store.Effective(*p).ProviderCfg(id))
				cfg.Enabled = true
				if err := reg.Enable(context.Background(), id, cfg); err != nil {
					return err
				}
			} else if err := reg.Disable(context.Background(), id); err != nil {
				return err
			}
			pc.Enabled = enabled
			p.SetProviderCfg(id, pc)
			return nil
		})
	}
}

// buildValidateModulePathFn returns a closure that validates a provider path,
// opens the provider with the validated config, and persists the path to the
// active profile in config.json.
func buildValidateModulePathFn(reg *providers.Registry, store *config.Store) func(ctx context.Context, id, path string) error {
	return func(ctx context.Context, id, path string) error {
		if store == nil {
			return errNoProfileStore
		}
		cfg := providers.ProviderConfig{Enabled: true, Path: path}
		if err := reg.Validate(ctx, id, cfg); err != nil {
			return err
		}
		return store.UpdateActive(func(_ string, p *config.Profile) error {
			// Validation passed — enable the provider with the new config.
			if err := reg.Enable(ctx, id, cfg); err != nil {
				return err
			}
			pc := p.ProviderCfg(id)
			pc.Enabled = true
			pc.Path = path
			p.SetProviderCfg(id, pc)
			return nil
		})
	}
}

// ---------------------------------------------------------------------------
// WU-9: Accounts page helpers — profile CRUD + switch wiring
// ---------------------------------------------------------------------------

// buildProfilesFn returns a closure that maps the store's profiles to
// []ui.ProfileInfo view-models for the Settings → Accounts page and switcher.
func buildProfilesFn(store *config.Store) func() []ui.ProfileInfo {
	return func() []ui.ProfileInfo {
		if store == nil {
			return nil
		}
		snap := store.Snapshot()
		// Stable order: default first, then alphabetical.
		names := make([]string, 0, len(snap.Profiles))
		for name := range snap.Profiles {
			names = append(names, name)
		}
		sortProfileNames(names)
		out := make([]ui.ProfileInfo, 0, len(names))
		for _, name := range names {
			out = append(out, ui.ProfileInfo{
				Name:   name,
				Active: name == snap.ActiveProfile,
			})
		}
		return out
	}
}

// sortProfileNames sorts profile names: "default" first, rest alphabetically.
func sortProfileNames(names []string) {
	// Simple insertion sort — profile count is tiny (< 20 in practice).
	for i := 1; i < len(names); i++ {
		for j := i; j > 0; j-- {
			a, b := names[j-1], names[j]
			if a == "default" {
				break
			}
			if b == "default" || a > b {
				names[j-1], names[j] = names[j], names[j-1]
			} else {
				break
			}
		}
	}
}

// buildActiveProfileFn returns a closure that reads the current active profile name.
func buildActiveProfileFn(store *config.Store) func() string {
	return func() string {
		if store == nil {
			return ""
		}
		return store.ActiveProfileName()
	}
}

// buildSwitchProfileFn returns a closure that switches the active profile in the
// registry and persists the new active profile name to config.json.
//
// Hot-swap semantics (design verdict 2):
//   - Opens next profile's provider instances BEFORE closing old (validate-first).
//   - On ANY open failure: aborts, keeps old profile active, returns error.
//   - On success: swaps handles, persists activeProfile atomically.
//   - A missing-path provider degrades to unavailable (not fatal) — design spec §5.
func buildSwitchProfileFn(reg *providers.Registry, store *config.Store) func(name string) error {
	return func(name string) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.SwitchActive(name, func(next config.Profile) error {
			if err := reg.SwitchProfile(context.Background(), configProfileAdapter{p: next}); err != nil {
				return fmt.Errorf("profile switch failed: %w", err)
			}
			return nil
		})
	}
}

// buildCreateProfileFn returns a closure that creates a new named profile in
// the store. The new profile starts empty (no provider configs). Returns an
// error if a profile with that name already exists.
func buildCreateProfileFn(store *config.Store) func(name string) error {
	return func(name string) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.Update(func(st *config.ProfileStore) error {
			if _, exists := st.Profiles[name]; exists {
				return usererr.New(fmt.Sprintf("profile already exists: %q", name))
			}
			st.Profiles[name] = config.Profile{}
			return nil
		})
	}
}

// buildDeleteProfileFn returns a closure that deletes the named profile from
// the store. Returns an error if:
//   - The profile is the currently active profile.
//   - It is the last remaining profile (must always have ≥1).
func buildDeleteProfileFn(store *config.Store) func(name string) error {
	return func(name string) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.Update(func(st *config.ProfileStore) error {
			if st.ActiveProfile == name {
				return usererr.New(fmt.Sprintf("cannot delete active profile %q; switch to another profile first", name))
			}
			if len(st.Profiles) <= 1 {
				return usererr.New("cannot delete the last remaining profile")
			}
			if _, exists := st.Profiles[name]; !exists {
				return usererr.New(fmt.Sprintf("profile %q not found", name))
			}
			delete(st.Profiles, name)
			return nil
		})
	}
}

// ---------------------------------------------------------------------------
// WU-11: CC account sources — multi-account read path
// ---------------------------------------------------------------------------

// buildCCAccountSourcesFn returns a closure that builds []services.CCAccountSource
// from the enabled CC accounts in the store (CLAUDE_PROJECTS_DIR, when set,
// replaces the path of the "default" account in memory). It never writes:
// config.OpenStore seeds the default account once at boot.
//
// Guard nil store: if the store is nil (legacy/no-registry paths that build Deps
// without a store), falls back to a single source using defaultClaudeDir.
// The configHome parameter is unused: the store knows where it persists.
func buildCCAccountSourcesFn(store *config.Store, defaultClaudeDir, _ string) func() []services.CCAccountSource {
	fallback := []services.CCAccountSource{{
		ID:     "default",
		Label:  "Personal",
		Reader: services.DiskProjectsReader{BaseDir: defaultClaudeDir},
	}}
	return func() []services.CCAccountSource {
		if store == nil {
			return fallback
		}
		var out []services.CCAccountSource
		for _, acc := range store.EffectiveCCAccounts() {
			if !acc.Enabled {
				continue
			}
			out = append(out, services.CCAccountSource{
				ID:     acc.ID,
				Label:  acc.Label,
				Reader: services.DiskProjectsReader{BaseDir: acc.Path},
			})
		}
		if len(out) == 0 {
			// No account or all disabled — fall back so the UI still shows something.
			return fallback
		}
		return out
	}
}

// ---------------------------------------------------------------------------
// Slice 2b: CC Accounts settings helpers
// ---------------------------------------------------------------------------

// buildCCAccountsFn returns a closure that maps all CC accounts in the store
// to []ui.CCAccountInfo view-models (enabled and disabled).
func buildCCAccountsFn(store *config.Store) func() []ui.CCAccountInfo {
	return func() []ui.CCAccountInfo {
		if store == nil {
			return nil
		}
		accounts := store.CCAccounts()
		out := make([]ui.CCAccountInfo, 0, len(accounts))
		for _, acc := range accounts {
			out = append(out, ui.CCAccountInfo{
				ID:      acc.ID,
				Label:   acc.Label,
				Path:    acc.Path,
				Enabled: acc.Enabled,
			})
		}
		return out
	}
}

// buildAddCCAccountFn returns a closure that validates a path via the
// cc-sessions provider and then creates a new CC account in the store.
func buildAddCCAccountFn(reg *providers.Registry, store *config.Store) func(label, path string) error {
	return func(label, path string) error {
		if store == nil {
			return errNoProfileStore
		}
		// Validate the path using the cc-sessions provider.
		cfg := providers.ProviderConfig{Enabled: false, Path: path}
		if err := reg.Validate(context.Background(), "cc-sessions", cfg); err != nil {
			return err
		}
		_, err := store.AddCCAccount(label, path)
		return err
	}
}

// buildUpdateCCAccountFn returns a closure that updates a CC account's
// label, path and enabled state and persists it through the store.
func buildUpdateCCAccountFn(store *config.Store) func(id, label, path string, enabled bool) error {
	return func(id, label, path string, enabled bool) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.UpdateCCAccount(id, label, path, enabled)
	}
}

// buildRemoveCCAccountFn returns a closure that removes a CC account from the
// store, which refuses to remove the last account.
func buildRemoveCCAccountFn(store *config.Store) func(id string) error {
	return func(id string) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.RemoveCCAccount(id)
	}
}

// ---------------------------------------------------------------------------
// PR6b: Advanced view toggle — per-profile preference
// ---------------------------------------------------------------------------

// buildAdvancedViewFn returns a closure that reads the AdvancedView preference
// from the active profile. Returns false when the store is nil or the profile
// does not exist.
func buildAdvancedViewFn(store *config.Store) func() bool {
	return func() bool {
		if store == nil {
			return false
		}
		_, p, ok := store.ActiveProfile()
		return ok && p.IsAdvancedView()
	}
}

// buildSetAdvancedViewFn returns a closure that sets the AdvancedView preference
// on the active profile and persists the change to config.json.
func buildSetAdvancedViewFn(store *config.Store) func(enabled bool) error {
	return func(enabled bool) error {
		if store == nil {
			return errNoProfileStore
		}
		return store.UpdateActive(func(_ string, p *config.Profile) error {
			p.Preferences.AdvancedView = enabled
			return nil
		})
	}
}

// ---------------------------------------------------------------------------
// ProviderConfig bridge (config.ProviderConfig → providers.ProviderConfig)
// ---------------------------------------------------------------------------

// providerConfigFrom converts a config.ProviderConfig (JSON persistence type
// from internal/config) to providers.ProviderConfig (port interface type from
// internal/providers). Both structs have identical fields; this shim exists
// because the two packages must remain import-cycle-free.
func providerConfigFrom(c config.ProviderConfig) providers.ProviderConfig {
	return providers.ProviderConfig{
		Enabled:   c.Enabled,
		Path:      c.Path,
		DaemonURL: c.DaemonURL,
	}
}

// configProfileAdapter adapts a config.Profile (which has ProviderCfg returning
// config.ProviderConfig) to the providers.Profile interface (which requires
// ProviderCfg returning providers.ProviderConfig). The adapter lives in httpapi
// so that config and providers remain import-cycle-free.
type configProfileAdapter struct {
	p config.Profile
}

// ProviderCfg satisfies providers.Profile.
func (a configProfileAdapter) ProviderCfg(id string) providers.ProviderConfig {
	return providerConfigFrom(a.p.ProviderCfg(id))
}

// AdaptProfile wraps a config.Profile as a providers.Profile. Used by main.go
// when calling registry.Boot(ctx, AdaptProfile(activeProfile)).
func AdaptProfile(p config.Profile) providers.Profile {
	return configProfileAdapter{p: p}
}
