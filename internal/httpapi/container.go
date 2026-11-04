package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// Container holds all shared dependencies for the HTTP server.
type Container struct {
	// Config is the immutable startup snapshot — do NOT write any field after
	// NewContainer returns (it is copied by value into Deps). Mutable paths live
	// in Paths instead, which is shared by pointer and lock-guarded.
	Config config.Config
	Paths  *config.RuntimePaths // live, lock-guarded source of truth for mutable paths
	RoDB   sqlite.Querier       // read-only pool (hot-swappable)
	RWDB   sqlite.Querier       // read-write pool (hot-swappable); nil when unavailable
	Logger *slog.Logger

	// Registry, when non-nil, is the provider registry used for data-driven
	// route mounting and health aggregation (WU-5+ path). When nil, the
	// container operates in legacy direct-mount mode (backward compat for tests
	// and callers that construct via NewContainer).
	Registry *providers.Registry

	// ProfileStore, when non-nil, is the lock-guarded owner of config.json and
	// the active profile, used by the Settings pages to read and write
	// per-provider enablement and paths. It is set from main.go when a registry
	// is used. Never access the underlying ProfileStore data directly.
	ProfileStore *config.Store

	// CloseGrace is how long ReloadEngramDB waits before closing the superseded
	// pools, letting in-flight queries drain. Tests set it to 0 to close
	// synchronously (so temp DB files unlock deterministically).
	CloseGrace time.Duration

	roSwap *sqlite.SwapDB // backing swappable handle for RoDB
	rwSwap *sqlite.SwapDB // backing swappable handle for RWDB; nil in read-only mode
	mu     sync.Mutex     // serialises ReloadEngramDB
}

// NewContainer opens the database pools and returns a Container.
//
// The read-only pool (RoDB) is required; a failure is fatal. It serves every
// read view and is opened with mode=ro + query_only so it can never mutate the
// database and coexists safely with the live WAL written by the daemon.
//
// The read-write pool (RWDB) is OPTIONAL and powers only the explicit,
// user-confirmed write routes (observation edits, project reassignment,
// deletes, project rename, db import/export). Cloud sync is read-only.
// It is left nil — disabling every write route via guardRWDB — when either:
//   - ENGRAM_DASH_READONLY is set (the operator opted into a pure read-only
//     viewer), or
//   - the writable handle cannot be opened (e.g. the DB file is not writable).
//
// Both pools are wrapped in a sqlite.SwapDB so the user can repoint the
// dashboard at a different database file at runtime (see ReloadEngramDB).
func NewContainer(cfg config.Config, logger *slog.Logger) (*Container, error) {
	roDB, err := sqlite.OpenReadOnly(cfg.EngramDbPath)
	if err != nil {
		return nil, err
	}
	roSwap := sqlite.NewSwapDB(roDB)

	var rwSwap *sqlite.SwapDB
	var rwQ sqlite.Querier // nil interface ⇒ write routes disabled
	if cfg.ReadOnly {
		logger.Info("read-only mode enabled (ENGRAM_DASH_READONLY); all write routes return 503")
	} else if rw, rwErr := sqlite.OpenReadWrite(cfg.EngramDbPath); rwErr != nil {
		logger.Warn("read-write database unavailable; write routes will be disabled",
			"path", cfg.EngramDbPath,
			"err", rwErr,
		)
	} else {
		rwSwap = sqlite.NewSwapDB(rw)
		rwQ = rwSwap
	}

	return &Container{
		Config:     cfg,
		Paths:      config.NewRuntimePaths(cfg.EngramDbPath, cfg.ClaudeProjectsDir),
		RoDB:       roSwap,
		RWDB:       rwQ,
		Logger:     logger,
		CloseGrace: 5 * time.Second,
		roSwap:     roSwap,
		rwSwap:     rwSwap,
	}, nil
}

// ReloadEngramDB validates newPath, opens fresh pools, swaps them in live, and
// saves the path into the active profile of config.json (through the Store,
// so an ephemeral demo store saves nothing). The previous pools are closed after a short grace so
// in-flight queries can drain. On any validation/open error the current DB is
// left untouched. Live reads of the path go through c.Paths; c.Config is the
// immutable startup snapshot and is not mutated here.
//
// newPath must pass engramprovider.ValidateDBFile (regular SQLite file with the
// Engram schema) — the same check the Settings Modules path applies. Returned
// errors are user-facing; OS/SQLite details go to the log only.
func (c *Container) ReloadEngramDB(newPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := engramprovider.ValidateDBFile(newPath); err != nil {
		c.Logger.Warn("rejected engram database path", "path", newPath, "err", err)
		return err
	}

	newRo, err := sqlite.OpenReadOnly(newPath)
	if err != nil {
		c.Logger.Warn("cannot open engram database read-only", "path", newPath, "err", err)
		return usererr.New(fmt.Sprintf("cannot open database: %s", newPath))
	}

	var newRw *sql.DB
	if !c.Config.ReadOnly {
		newRw, err = sqlite.OpenReadWrite(newPath)
		if err != nil {
			newRo.Close()
			c.Logger.Warn("cannot open engram database read-write", "path", newPath, "err", err)
			return usererr.New(fmt.Sprintf("database is not writable: %s", newPath))
		}
	}

	oldRo := c.roSwap.Swap(newRo)
	var oldRw *sql.DB
	switch {
	case c.rwSwap != nil && newRw != nil:
		oldRw = c.rwSwap.Swap(newRw)
	case newRw != nil:
		// Started without a writable pool (read-only mode / earlier open failure).
		// Deps captured a nil RWDB, so writes cannot be retroactively enabled
		// without a restart; discard the freshly opened writable pool.
		newRw.Close()
	}

	c.Paths.SetEngramDB(newPath)

	// config.json is the only persisted configuration source (the legacy
	// explorer-settings.json is no longer written). Containers built without a
	// store (legacy tests) apply the change live only.
	if c.ProfileStore != nil {
		err := c.ProfileStore.UpdateActive(func(_ string, p *config.Profile) error {
			ec := p.ProviderCfg("engram")
			ec.Enabled = true
			ec.Path = newPath
			p.SetProviderCfg("engram", ec)
			return nil
		})
		if err != nil {
			c.Logger.Warn("failed to persist engram path to active profile", "err", err)
		}
	}

	// Close the old pools after a grace period so in-flight queries drain.
	closeOld := func() {
		if oldRo != nil {
			oldRo.Close()
		}
		if oldRw != nil {
			oldRw.Close()
		}
	}
	if c.CloseGrace <= 0 {
		closeOld()
	} else {
		grace := c.CloseGrace
		go func() {
			time.Sleep(grace)
			closeOld()
		}()
	}

	c.Logger.Info("reloaded engram database", "path", newPath)
	return nil
}

// SetClaudeDir validates and applies a new Claude Code projects directory. There
// is no persistent connection to swap — transcripts are scanned per request — so
// the change takes effect immediately for every reader of c.Paths.ClaudeDir().
// The directory is saved as the cc-sessions path of the active profile in
// config.json (through the Store; never when the store is ephemeral).
//
// newDir must pass ccsessions.ValidateProjectsDir — the same check the Settings
// Modules path applies. Returned errors are user-facing.
func (c *Container) SetClaudeDir(newDir string) error {
	if err := ccsessions.ValidateProjectsDir(newDir); err != nil {
		c.Logger.Warn("rejected claude projects directory", "path", newDir, "err", err)
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.Paths.SetClaudeDir(newDir)
	if c.ProfileStore != nil {
		err := c.ProfileStore.UpdateActive(func(_ string, p *config.Profile) error {
			cc := p.ProviderCfg("cc-sessions")
			cc.Path = newDir
			p.SetProviderCfg("cc-sessions", cc)
			return nil
		})
		if err != nil {
			c.Logger.Warn("failed to persist claude projects dir to active profile", "err", err)
		}
	}
	return nil
}

// Close releases database connections and shuts down all active provider
// instances (when the container is registry-backed). Safe to call multiple times.
func (c *Container) Close() {
	if c.roSwap != nil {
		if db := c.roSwap.Current(); db != nil {
			db.Close()
		}
	}
	if c.rwSwap != nil {
		if db := c.rwSwap.Current(); db != nil {
			db.Close()
		}
	}
	// In registry-backed mode the DB handles are owned by the provider
	// instances — close them via the registry to release file locks.
	if c.Registry != nil {
		c.Registry.Shutdown(context.Background())
	}
}

// NewContainerWithRegistry creates a Container backed by a provider Registry.
// Unlike NewContainer it does NOT call sqlite.OpenReadOnly at construction
// time — database handles are owned by the registry's Engram provider instance.
// Boot (registry.Boot) must be called BEFORE NewContainerWithRegistry so that
// provider instances are already open; this constructor only links the registry
// into the container for route mounting and health aggregation.
//
// When the Engram provider is active in the registry, RoDB/RWDB are populated
// from its instance's Deps (via the engramDepsAccessor interface) so that
// legacy route handlers still work. When Engram is absent (zero providers),
// RoDB/RWDB remain nil — which is the correct non-fatal-boot state.
func NewContainerWithRegistry(reg *providers.Registry, cfg config.Config, logger *slog.Logger) *Container {
	c := &Container{
		Config:     cfg,
		Paths:      config.NewRuntimePaths(cfg.EngramDbPath, cfg.ClaudeProjectsDir),
		Logger:     logger,
		Registry:   reg,
		CloseGrace: 5 * time.Second,
	}

	if reg != nil {
		populateFromRegistry(c, reg)
	}

	return c
}

// engramDepsAccessor is a narrow interface satisfied by the Engram provider's
// engramInstance. It exposes the DB handles and RuntimePaths needed to
// populate Container fields without importing internal/providers/engram
// (avoiding any import-cycle risk in container.go).
//
// The Engram provider implements this interface via its ContainerDeps() method.
type engramDepsAccessor interface {
	ContainerDeps() (roDB sqlite.Querier, rwDB sqlite.Querier, paths *config.RuntimePaths)
}

// populateFromRegistry inspects the registry for an active Engram provider
// instance and extracts its DB handles and RuntimePaths into the container's
// legacy fields (RoDB, RWDB, Paths). This keeps existing route handlers and
// tests that read c.RoDB / c.Paths working on the new registry-based path.
func populateFromRegistry(c *Container, reg *providers.Registry) {
	for _, entry := range reg.Entries() {
		if entry.State != providers.Enabled {
			continue
		}
		inst := reg.Instance(entry.ID)
		if inst == nil {
			continue
		}
		if ea, ok := inst.(engramDepsAccessor); ok {
			roDB, rwDB, paths := ea.ContainerDeps()
			c.RoDB = roDB
			c.RWDB = rwDB
			if paths != nil {
				c.Paths = paths
			}
			// Recover the concrete SwapDB handles so ReloadEngramDB can hot-swap
			// the very pools the handlers read through. The Engram provider wraps
			// both pools in *sqlite.SwapDB and hands them to the container as the
			// sqlite.Querier interface; without these assignments c.roSwap/c.rwSwap
			// stay nil on the registry path and ReloadEngramDB panics on swap
			// (the in-app DB-path switcher "does nothing"/crashes bug).
			if sw, ok := roDB.(*sqlite.SwapDB); ok {
				c.roSwap = sw
			}
			if sw, ok := rwDB.(*sqlite.SwapDB); ok {
				c.rwSwap = sw
			}
		}
	}
}
