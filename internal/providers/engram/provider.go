// Package engram implements the Provider port for the Engram SQLite data source.
// It wraps the existing internal/sqlite pools, internal/ui and internal/doctor
// modules, and the daemon client — all behind the providers.Provider interface.
// The actual handlers are NOT rewritten: Routes calls the same route-group
// functions that server.go already calls, moved verbatim (incremental adapter
// strategy from design decision 1).
package engram

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	_ "modernc.org/sqlite" // register the "sqlite" driver

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/doctor"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// SchemaValid — shared by Detect, Validate, and Health (single source of truth)
// ---------------------------------------------------------------------------

// SchemaValid returns true when db contains both the 'observations' and
// 'sessions' tables, confirming it is a valid Engram database file.
//
// This replaces the bare "SELECT 1" check in the existing healthHandler, which
// always returned true for an empty/schemaless SQLite file (false-positive fix
// from spec requirement: health-reporting / Schema-Validity Check).
func SchemaValid(db *sql.DB) bool {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('observations','sessions')`,
	).Scan(&n)
	return err == nil && n >= 2
}

// ---------------------------------------------------------------------------
// engramProvider — implements providers.Provider
// ---------------------------------------------------------------------------

// engramProvider is the singleton descriptor for the Engram data source.
type engramProvider struct {
	cfg config.Config // base app config (DaemonBaseURL, DaemonTimeoutMs, etc.)
}

// NewProvider returns the Engram Provider adapter, configured from the base
// application config. The returned value is a singleton intended to be
// registered once with the Registry.
func NewProvider(cfg config.Config) providers.Provider {
	return &engramProvider{cfg: cfg}
}

// ID returns the stable machine identifier used as the registry key, config
// map key, and URL slug.
func (p *engramProvider) ID() string { return "engram" }

// DisplayName returns the human-readable label. i18n resolution happens in the
// templ layer; the provider returns the English base string.
func (p *engramProvider) DisplayName() string { return "Engram" }

// ProviderTier returns Tier1: Engram is a first-class provider that stays
// visible in Settings when not auto-detected, offering manual path entry.
func (p *engramProvider) ProviderTier() providers.Tier { return providers.Tier1 }

// Featured returns true: Engram is always rendered first and visually
// featured in the sidebar and Settings Modules page.
func (p *engramProvider) Featured() bool { return true }

// Detect reports whether a valid Engram database exists at cfg.Path.
// It is a pure check: (1) file exists, (2) schema is valid.
// It MUST NOT open long-lived handles — it opens a read-only connection,
// runs the sqlite_master query, and closes immediately.
func (p *engramProvider) Detect(ctx context.Context, cfg providers.ProviderConfig) providers.Detection {
	path := cfg.Path
	if path == "" {
		return providers.Detection{Available: false, Reason: "path-missing", Path: path}
	}

	if _, err := os.Stat(path); err != nil {
		return providers.Detection{Available: false, Reason: "path-missing", Path: path}
	}

	// Open read-only, check schema, close immediately — no long-lived handle.
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return providers.Detection{Available: false, Reason: "schema-invalid", Path: path}
	}
	valid := SchemaValid(db)
	db.Close()

	if !valid {
		return providers.Detection{Available: false, Reason: "schema-invalid", Path: path}
	}
	return providers.Detection{Available: true, Reason: "ok", Path: path}
}

// Open acquires the SQLite pools (read-only + read-write) for cfg.Path and
// returns an engramInstance. It reuses the same sqlite.OpenReadOnly /
// sqlite.OpenReadWrite functions used by NewContainer today.
//
// Open returns an error when the DB file is absent or cannot be opened. The
// registry treats an Open error as Errored state and continues booting other
// providers — it is NEVER fatal.
func (p *engramProvider) Open(ctx context.Context, cfg providers.ProviderConfig) (providers.Instance, error) {
	path := cfg.Path
	if path == "" {
		return nil, fmt.Errorf("engram: path is required")
	}

	roDB, err := sqlite.OpenReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("engram: open read-only pool: %w", err)
	}

	roSwap := sqlite.NewSwapDB(roDB)

	var rwSwap *sqlite.SwapDB
	var rwQ sqlite.Querier
	if !p.cfg.ReadOnly {
		if rw, rwErr := sqlite.OpenReadWrite(path); rwErr != nil {
			// Read-write unavailable — write routes are disabled but the provider
			// still opens successfully (same behaviour as NewContainer today).
		} else {
			rwSwap = sqlite.NewSwapDB(rw)
			rwQ = rwSwap
		}
	}

	appCfg := p.cfg
	appCfg.EngramDbPath = path

	runtimePaths := config.NewRuntimePaths(path, appCfg.ClaudeProjectsDir)

	deps := ui.Deps{
		RoDB:   roSwap,
		RWDB:   rwQ,
		Config: appCfg,
		Paths:  runtimePaths,
	}

	return &engramInstance{
		deps:   deps,
		roSwap: roSwap,
		rwSwap: rwSwap,
		path:   path,
	}, nil
}

// Routes registers the Engram HTTP routes on mux. It calls the SAME
// route-group functions that server.go calls today, moved verbatim
// (incremental adapter strategy — no handler rewrites).
func (p *engramProvider) Routes(mux *http.ServeMux, inst providers.Instance, logger *slog.Logger) {
	// Routes is intentionally a no-op in WU-3 because the route-group functions
	// (observationsRoutes, promptsRoutes, etc.) live in internal/httpapi which
	// imports internal/providers — importing httpapi here would create an import
	// cycle. The actual route wiring moves to WU-5 (container wiring) where the
	// container owns both the registry and the route-group functions.
	//
	// This stub satisfies the Provider interface contract so the adapter can be
	// registered and tested in isolation. WU-5 will pass the real route
	// registration through the container.
}

// Nav returns the sidebar NavGroup for the Engram provider.
func (p *engramProvider) Nav(inst providers.Instance) providers.NavGroup {
	return providers.NavGroup{
		ID:       "engram",
		LabelKey: "nav.engram",
		Featured: true,
		Links: []providers.NavLink{
			{Href: "/", Key: "overview", LabelKey: "nav.overview"},
			{Href: "/observations", Key: "observations", LabelKey: "nav.observations"},
			{Href: "/sessions", Key: "sessions", LabelKey: "nav.sessions"},
			{Href: "/prompts", Key: "prompts", LabelKey: "nav.prompts"},
			{Href: "/topics", Key: "topics", LabelKey: "nav.topics"},
			{Href: "/projects", Key: "projects", LabelKey: "nav.projects"},
			{Href: "/brain", Key: "brain", LabelKey: "nav.brain"},
		},
	}
}

// Validate checks a candidate config before activation (Tier-1 manual path
// entry). For Engram: (1) file must exist, (2) schema must be valid.
func (p *engramProvider) Validate(ctx context.Context, cfg providers.ProviderConfig) error {
	path := cfg.Path
	if path == "" {
		return fmt.Errorf("engram: path is required")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("engram: database file not found at %q: %w", path, err)
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return fmt.Errorf("engram: cannot open database for validation: %w", err)
	}
	defer db.Close()

	if !SchemaValid(db) {
		return fmt.Errorf("engram: database at %q is not a valid Engram DB (missing observations or sessions tables)", path)
	}
	return nil
}

// ---------------------------------------------------------------------------
// engramInstance — implements providers.Instance + providers.Writable
// ---------------------------------------------------------------------------

// engramInstance is the per-profile live activation of the Engram provider.
// It holds the SQLite pools and the Deps struct that the existing handlers
// already consume — nothing changes below the adapter seam.
type engramInstance struct {
	deps   ui.Deps
	roSwap *sqlite.SwapDB
	rwSwap *sqlite.SwapDB // nil in read-only mode
	path   string
}

// Health returns per-provider health using SchemaValid — not bare SELECT 1.
// This is what fixes the false-positive: an empty/schemaless DB now reports
// ok=false.
func (i *engramInstance) Health(ctx context.Context) providers.ProviderHealth {
	h := providers.ProviderHealth{
		ID:   "engram",
		Path: i.path,
	}

	roDB := i.deps.RoDB
	if roDB == nil {
		h.Error = "no read-only pool"
		return h
	}

	// Type-assert to *sqlite.SwapDB to get the underlying *sql.DB for SchemaValid.
	// If the assertion fails we fall back to a raw ping.
	if swapDB, ok := roDB.(*sqlite.SwapDB); ok {
		db := swapDB.Current()
		if db == nil {
			h.Error = "pool is nil"
			return h
		}
		h.OK = SchemaValid(db)
		if !h.OK {
			h.Error = "schema not found"
		}
		return h
	}

	// Fallback: try the sqlite_master query directly via the Querier interface.
	var n int
	err := roDB.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('observations','sessions')`,
	).Scan(&n)
	h.OK = err == nil && n >= 2
	if !h.OK {
		if err != nil {
			h.Error = err.Error()
		} else {
			h.Error = "schema not found"
		}
	}
	return h
}

// Close releases the SQLite pools. Called by the registry on profile switch
// or server shutdown.
func (i *engramInstance) Close(ctx context.Context) error {
	var firstErr error
	if i.roSwap != nil {
		if db := i.roSwap.Current(); db != nil {
			if err := db.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if i.rwSwap != nil {
		if db := i.rwSwap.Current(); db != nil {
			if err := db.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Writable returns true — Engram is the only provider that accepts write
// operations. This satisfies the providers.Writable optional capability.
func (i *engramInstance) Writable() bool { return true }

// Deps returns the ui.Deps struct held by this instance. Used by WU-5
// (container wiring) to mount routes without an import cycle.
func (i *engramInstance) Deps() ui.Deps { return i.deps }

// DoctorDeps returns a doctor.Deps struct built from this instance's fields.
// Used by WU-5 (container wiring) to mount doctor routes.
func (i *engramInstance) DoctorDeps() doctor.Deps {
	return doctor.Deps{
		RoDB:   i.deps.RoDB,
		RWDB:   i.deps.RWDB,
		Config: i.deps.Config,
		Paths:  i.deps.Paths,
	}
}

// ContainerDeps satisfies the httpapi.engramDepsAccessor interface. It returns
// the DB queriers and RuntimePaths that httpapi.Container populates its own
// fields from when operating in registry-backed mode (WU-5+). This keeps
// existing route handlers (which read c.RoDB / c.RWDB / c.Paths directly)
// working without modification.
func (i *engramInstance) ContainerDeps() (roDB sqlite.Querier, rwDB sqlite.Querier, paths *config.RuntimePaths) {
	return i.deps.RoDB, i.deps.RWDB, i.deps.Paths
}

// ---------------------------------------------------------------------------
// Package-level accessor helpers for WU-5 (container wiring)
// ---------------------------------------------------------------------------

// EngramDeps extracts the ui.Deps and doctor.Deps from a providers.Instance
// that is an active Engram instance. Returns (ui.Deps, doctor.Deps, true) on
// success, or zero values and false if inst is not an Engram instance.
//
// This helper lets httpapi/server.go wire Engram routes without importing the
// unexported engramInstance type directly.
func EngramDeps(inst providers.Instance) (ui.Deps, doctor.Deps, bool) {
	ei, ok := inst.(*engramInstance)
	if !ok {
		return ui.Deps{}, doctor.Deps{}, false
	}
	return ei.Deps(), ei.DoctorDeps(), true
}
