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

	// Apply middleware stack (outermost = first to run).
	var handler http.Handler = mux
	handler = corsMiddleware(handler)
	handler = requestLogger(c.Logger, handler)
	handler = secureHeaders(handler)

	return handler
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

	// --- Phase 4 cloud CLI routes ---
	cloudRoutes(mux, c)

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

	// Resolve active profile name for WU-8 Modules callbacks.
	activeProfileName := "default"
	if c.ProfileStore != nil && c.ProfileStore.ActiveProfile != "" {
		activeProfileName = c.ProfileStore.ActiveProfile
	}

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
				cloudRoutes(mux, c)

				doctor.Mount(mux, ddeps)

				// Wire the container's ReloadEngramDB / SetClaudeDir callbacks so
				// the existing settings handlers (POST /settings/engram-db etc.)
				// keep working on the registry-backed path.
				deps.ReloadEngramDB = c.ReloadEngramDB
				deps.SetClaudeDir = c.SetClaudeDir
				// Wire NavGroups so the sidebar is data-driven from the registry.
				deps.NavGroups = reg.NavGroups
				// Wire WU-8 Modules callbacks.
				deps.Modules = buildModulesFn(reg, c.ProfileStore, activeProfileName)
				deps.ToggleModule = buildToggleModuleFn(reg, c.ProfileStore, activeProfileName, c.Config.ConfigHome)
				deps.ValidateModulePath = buildValidateModulePathFn(reg, c.ProfileStore, activeProfileName, c.Config.ConfigHome)
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
			ReloadEngramDB:     c.ReloadEngramDB,
			SetClaudeDir:       c.SetClaudeDir,
			NavGroups:          reg.NavGroups,
			Modules:            buildModulesFn(reg, c.ProfileStore, activeProfileName),
			ToggleModule:       buildToggleModuleFn(reg, c.ProfileStore, activeProfileName, c.Config.ConfigHome),
			ValidateModulePath: buildValidateModulePathFn(reg, c.ProfileStore, activeProfileName, c.Config.ConfigHome),
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
// []ui.ModuleInfo view-models for the Settings → Modules page. The ProfileStore
// (may be nil) is used to read the current path for each provider.
func buildModulesFn(reg *providers.Registry, store *config.ProfileStore, activeProfileName string) func() []ui.ModuleInfo {
	return func() []ui.ModuleInfo {
		metas := reg.AllProviderMetas()
		out := make([]ui.ModuleInfo, 0, len(metas))
		for _, m := range metas {
			var path string
			if store != nil {
				if p, ok := store.Profiles[activeProfileName]; ok {
					path = p.ProviderCfg(m.ID).Path
				}
			}
			var errMsg string
			if m.Err != nil {
				errMsg = m.Err.Error()
			}
			out = append(out, ui.ModuleInfo{
				ID:          m.ID,
				DisplayName: m.DisplayName,
				Tier:        registryTierToUITier(m.Tier),
				State:       registryStateToUIState(m.State),
				Path:        path,
				Err:         errMsg,
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

// buildToggleModuleFn returns a closure that enables or disables a provider in
// the registry and persists the change to the active profile in config.json.
func buildToggleModuleFn(reg *providers.Registry, store *config.ProfileStore, activeProfileName, configHome string) func(id string, enabled bool) error {
	return func(id string, enabled bool) error {
		if store == nil {
			return fmt.Errorf("profile store not available")
		}

		// Get current profile config for this provider.
		profile, ok := store.Profiles[activeProfileName]
		if !ok {
			return fmt.Errorf("active profile %q not found", activeProfileName)
		}
		cfg := providerConfigFrom(profile.ProviderCfg(id))

		if enabled {
			cfg.Enabled = true
			if err := reg.Enable(context.Background(), id, cfg); err != nil {
				return err
			}
		} else {
			if err := reg.Disable(context.Background(), id); err != nil {
				return err
			}
		}

		// Persist the updated enablement to the active profile.
		if profile.Providers == nil {
			profile.Providers = make(map[string]config.ProviderConfig)
		}
		existing := profile.Providers[id]
		existing.Enabled = enabled
		profile.Providers[id] = existing
		store.Profiles[activeProfileName] = profile
		return config.SaveProfileStore(configHome, store)
	}
}

// buildValidateModulePathFn returns a closure that validates a provider path,
// opens the provider with the validated config, and persists the path to the
// active profile in config.json.
func buildValidateModulePathFn(reg *providers.Registry, store *config.ProfileStore, activeProfileName, configHome string) func(ctx context.Context, id, path string) error {
	return func(ctx context.Context, id, path string) error {
		if store == nil {
			return fmt.Errorf("profile store not available")
		}

		cfg := providers.ProviderConfig{Enabled: true, Path: path}
		if err := reg.Validate(ctx, id, cfg); err != nil {
			return err
		}

		// Validation passed — enable the provider with the new config.
		if err := reg.Enable(ctx, id, cfg); err != nil {
			return err
		}

		// Persist path + enabled to the active profile.
		profile, ok := store.Profiles[activeProfileName]
		if !ok {
			return fmt.Errorf("active profile %q not found", activeProfileName)
		}
		if profile.Providers == nil {
			profile.Providers = make(map[string]config.ProviderConfig)
		}
		existing := profile.Providers[id]
		existing.Enabled = true
		existing.Path = path
		profile.Providers[id] = existing
		store.Profiles[activeProfileName] = profile
		return config.SaveProfileStore(configHome, store)
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
