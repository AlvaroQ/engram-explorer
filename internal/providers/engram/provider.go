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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	_ "modernc.org/sqlite" // register the "sqlite" driver

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/doctor"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
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

// sqliteMagic is the 16-byte header every SQLite 3 database file starts with.
const sqliteMagic = "SQLite format 3\x00"

// ValidateDBFile checks that path is a usable Engram database before anything
// is pointed at it: a regular file that starts with the SQLite header, opens
// read-only, and passes SchemaValid. It is the single check shared by the
// provider's Validate (Settings Modules path) and the Settings DB switcher.
//
// Error messages are user-facing (usererr): they name only the path the caller
// supplied and never wrap the underlying OS or SQLite error text.
func ValidateDBFile(path string) error {
	if path == "" {
		return usererr.New("database path is required")
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return usererr.New(fmt.Sprintf("database file not found: %s", path))
	}
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot access database file: %s", path))
	}
	if !info.Mode().IsRegular() {
		return usererr.New(fmt.Sprintf("not a regular file: %s", path))
	}

	f, err := os.Open(path)
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot read database file: %s", path))
	}
	header := make([]byte, len(sqliteMagic))
	_, err = io.ReadFull(f, header)
	f.Close()
	if err != nil || string(header) != sqliteMagic {
		return usererr.New(fmt.Sprintf("not an SQLite database: %s", path))
	}

	// Same busy_timeout-aware read-only DSN as Detect, resilient against WAL
	// checkpoint pressure from the Engram daemon.
	db, err := sql.Open("sqlite", checkRODSN(path))
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot open database: %s", path))
	}
	defer db.Close()
	if !SchemaValid(db) {
		return usererr.New(fmt.Sprintf("not an Engram database (missing observations or sessions tables): %s", path))
	}
	return nil
}

// checkRODSN returns the SQLite DSN for one-shot read-only schema checks used
// by Detect and Validate. It includes the same busy_timeout(5000) as
// internal/sqlite/pool.go's OpenReadOnly to avoid SQLITE_BUSY during WAL
// checkpoint windows when the Engram daemon is actively writing. Using a bare
// "file:%s?mode=ro" DSN (without busy_timeout) is the regression introduced in
// WU-3/WU-6 that could cause Detect to report schema-invalid on a valid hot-WAL
// database, making the provider boot into Detected (not Enabled) state.
func checkRODSN(path string) string {
	return fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
}

// ---------------------------------------------------------------------------
// engramProvider — implements providers.Provider
// ---------------------------------------------------------------------------

// engramProvider is the singleton descriptor for the Engram data source.
type engramProvider struct {
	cfg config.Config // base app config (DaemonBaseURL, DaemonTimeoutMs, etc.)
	log *slog.Logger  // registry logger (SetLogger); nil logs nothing
}

// NewProvider returns the Engram Provider adapter, configured from the base
// application config. The returned value is a singleton intended to be
// registered once with the Registry.
func NewProvider(cfg config.Config) providers.Provider {
	return &engramProvider{cfg: cfg}
}

// SetLogger receives the registry logger (providers.LoggerSetter); instances
// opened afterwards log their health transitions through it.
func (p *engramProvider) SetLogger(log *slog.Logger) { p.log = log }

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

	// Open read-only with busy_timeout to match pool.go's open parameters.
	// Using a bare "mode=ro" DSN (no busy_timeout) can return SQLITE_BUSY during
	// WAL checkpoint windows when the Engram daemon is actively writing, making
	// Detect report schema-invalid on an otherwise valid database.
	db, err := sql.Open("sqlite", checkRODSN(path))
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
		health: providers.NewHealthLog(p.log),
	}, nil
}

// Routes registers the routes the Engram provider owns: the doctor module
// (/doctor/*, /settings/maintenance). The host gates them by this provider's
// state, and every request reads the pools of the instance pinned for it
// through r.Live(req), so a toggle or profile switch never leaves a route on a
// closed pool.
//
// The Engram JSON API (/api/observations, ...) and the memory pages stay in
// internal/httpapi and internal/ui (importing them here would be an import
// cycle); the host serves them from the capabilities of the live instance
// (Searcher, StatsSource, Writable and the SQLite pools).
func (p *engramProvider) Routes(r providers.Router) {
	doctor.Mount(r, doctor.Deps{
		Config: p.cfg,
		Live: func(req *http.Request) (ro, rw sqlite.Querier) {
			if ei, ok := r.Live(req).(*engramInstance); ok {
				return ei.handles()
			}
			return nil, nil
		},
	})
}

// Nav returns the sidebar NavGroup for the Engram provider.
// sessions, topics, and prompts are intentionally omitted from the primary nav
// (PR1 UX simplification). They remain accessible via direct URL and from the
// project detail page ("View all sessions →").
func (p *engramProvider) Nav(inst providers.Instance) providers.NavGroup {
	return providers.NavGroup{
		ID:       "engram",
		LabelKey: "nav.engram",
		Featured: true,
		Icon:     "/static/engram.png",
		Links: []providers.NavLink{
			{Href: "/brain", Key: "brain", LabelKey: "nav.brain", Icon: "brain"},
			{Href: "/", Key: "overview", LabelKey: "nav.overview", Icon: "overview"},
			{Href: "/observations", Key: "observations", LabelKey: "nav.observations", Icon: "observations"},
			{Href: "/projects", Key: "projects", LabelKey: "nav.projects", Icon: "projects"},
		},
	}
}

// Validate checks a candidate config before activation (Tier-1 manual path
// entry). It delegates to ValidateDBFile so the Modules path and the Settings
// DB switcher apply the same check.
func (p *engramProvider) Validate(ctx context.Context, cfg providers.ProviderConfig) error {
	if err := ValidateDBFile(cfg.Path); err != nil {
		return fmt.Errorf("engram: %w", err)
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
	health *providers.HealthLog
}

// Health returns per-provider health using SchemaValid — not bare SELECT 1.
// This is what fixes the false-positive: an empty/schemaless DB now reports
// ok=false. Failures are logged through the registry logger once per
// transition (see providers.HealthLog), not on every polled call.
func (i *engramInstance) Health(ctx context.Context) (h providers.ProviderHealth) {
	// The path comes from the live RuntimePaths: a Settings DB switch
	// (ReloadEngramDB) swaps the pools of this instance and updates it.
	h = providers.ProviderHealth{
		ID:   "engram",
		Path: i.livePath(),
	}
	var detail error // internal cause, logged but never shown in production
	defer func() {
		if h.OK {
			i.health.Healthy("engram database healthy again", "path", h.Path)
			return
		}
		args := []any{"path", h.Path, "reason", h.Error}
		if detail != nil {
			args = append(args, "err", detail)
		}
		i.health.Failing("engram health check failed", args...)
	}()

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
			// The driver error can name the file; production shows a generic
			// message and logs the detail (deferred above).
			detail = err
			h.Error = "database unavailable"
			if i.deps.Config.ExposeDetails {
				h.Error = err.Error()
			}
		} else {
			h.Error = "schema not found"
		}
	}
	return h
}

// livePath returns the database path currently served by this instance.
func (i *engramInstance) livePath() string {
	if i.deps.Paths != nil {
		if p := i.deps.Paths.EngramDB(); p != "" {
			return p
		}
	}
	return i.path
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

// Writable reports whether this instance holds a read-write pool (false in
// read-only mode or when the database file is not writable). It satisfies the
// providers.Writable capability the host uses to gate write routes.
func (i *engramInstance) Writable() bool { return i.rwSwap != nil }

// handles returns the read-only pool and, when Writable, the read-write pool.
func (i *engramInstance) handles() (ro, rw sqlite.Querier) {
	if i.Writable() {
		return i.deps.RoDB, i.deps.RWDB
	}
	return i.deps.RoDB, nil
}

// Search runs the full-text observation search (providers.Searcher); it backs
// GET /api/observations/search. The result is []services.ObservationWithSnippet.
func (i *engramInstance) Search(_ context.Context, q string, limit int) (any, error) {
	return services.ObservationsSearch(i.deps.RoDB, q, limit)
}

// Stats builds the dashboard overview (providers.StatsSource); it backs
// GET /api/overview. The result is *services.OverviewResponse.
func (i *engramInstance) Stats(_ context.Context) (any, error) {
	return services.OverviewBuild(i.deps.RoDB)
}

// Deps returns the ui.Deps struct held by this instance. Used by WU-5
// (container wiring) to mount routes without an import cycle.
func (i *engramInstance) Deps() ui.Deps { return i.deps }

// ContainerDeps satisfies the httpapi.engramDepsAccessor interface. It returns
// the DB queriers and RuntimePaths that httpapi.Container populates its own
// fields from when operating in registry-backed mode (WU-5+). This keeps
// existing route handlers (which read c.RoDB / c.RWDB / c.Paths directly)
// working without modification.
func (i *engramInstance) ContainerDeps() (roDB sqlite.Querier, rwDB sqlite.Querier, paths *config.RuntimePaths) {
	return i.deps.RoDB, i.deps.RWDB, i.deps.Paths
}
