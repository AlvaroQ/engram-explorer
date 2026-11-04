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
// so an ephemeral store saves nothing). The previous pools are closed after a short grace so
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

	roSwap, rwSwap, instPaths := c.liveSwaps()
	if roSwap == nil {
		newRo.Close()
		if newRw != nil {
			newRw.Close()
		}
		return usererr.New("the Engram module is disabled; enable it in Settings → Modules first")
	}

	oldRo := roSwap.Swap(newRo)
	var oldRw *sql.DB
	switch {
	case rwSwap != nil && newRw != nil:
		oldRw = rwSwap.Swap(newRw)
	case newRw != nil:
		// Started without a writable pool (read-only mode / earlier open failure).
		// The instance stays read-only until it is reopened (toggle or restart);
		// discard the freshly opened writable pool.
		newRw.Close()
	}

	c.Paths.SetEngramDB(newPath)
	if instPaths != nil && instPaths != c.Paths {
		instPaths.SetEngramDB(newPath) // the live instance reports it in /api/health
	}

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

// liveSwaps returns the swappable pools ReloadEngramDB swaps: the container's
// own in legacy mode, the live store instance's in registry mode (nil when
// the Engram module is disabled), plus that instance's RuntimePaths.
func (c *Container) liveSwaps() (ro, rw *sqlite.SwapDB, paths *config.RuntimePaths) {
	if c.Registry == nil {
		return c.roSwap, c.rwSwap, nil
	}
	inst, ok := c.Registry.Find(isSQLiteStore).(sqliteStore)
	if !ok {
		return nil, nil, nil
	}
	roQ, rwQ, paths := inst.ContainerDeps()
	ro, _ = roQ.(*sqlite.SwapDB)
	rw, _ = rwQ.(*sqlite.SwapDB)
	return ro, rw, paths
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
// time — database handles are owned by the registry's provider instances.
// Boot (registry.Boot) must be called BEFORE NewContainerWithRegistry so that
// provider instances are already open.
//
// RoDB/RWDB are liveDB queriers: every call goes to the pool of the store
// instance that is live at that moment (none when the Engram module is off),
// so enabling, disabling or switching profiles never leaves a handler on a
// closed pool.
func NewContainerWithRegistry(reg *providers.Registry, cfg config.Config, logger *slog.Logger) *Container {
	c := &Container{
		Config:     cfg,
		Paths:      config.NewRuntimePaths(cfg.EngramDbPath, cfg.ClaudeProjectsDir),
		Logger:     logger,
		Registry:   reg,
		CloseGrace: 5 * time.Second,
	}

	if reg != nil {
		c.RoDB = liveDB{c: c}
		c.RWDB = liveDB{c: c, rw: true}
		populateFromRegistry(c, reg)
	}

	return c
}

// populateFromRegistry seeds the container's live paths from the instances
// enabled at boot: the database path of the SQLite store and the projects
// directory of the transcript source.
func populateFromRegistry(c *Container, reg *providers.Registry) {
	if st, ok := reg.Find(isSQLiteStore).(sqliteStore); ok {
		if _, _, paths := st.ContainerDeps(); paths != nil {
			c.Paths.SetEngramDB(paths.EngramDB())
		}
	}
	if ts, ok := reg.Find(isTranscriptSource).(transcriptSource); ok {
		if paths := ts.RuntimePaths(); paths != nil {
			c.Paths.SetClaudeDir(paths.ClaudeDir())
		}
	}
}
