package httpapi

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
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
// deletes, project rename, db import/export, and the local cloud unenroll).
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
// persists the override. The previous pools are closed after a short grace so
// in-flight queries can drain. On any validation/open error the current DB is
// left untouched. Live reads of the path go through c.Paths; c.Config is the
// immutable startup snapshot and is not mutated here.
func (c *Container) ReloadEngramDB(newPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	newRo, err := sqlite.OpenReadOnly(newPath)
	if err != nil {
		return err
	}

	var newRw *sql.DB
	if !c.Config.ReadOnly {
		newRw, err = sqlite.OpenReadWrite(newPath)
		if err != nil {
			newRo.Close()
			return fmt.Errorf("database is not writable: %w", err)
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
	if err := config.SaveOverrides(c.Config.ConfigHome, config.Overrides{
		EngramDbPath:      newPath,
		ClaudeProjectsDir: c.Paths.ClaudeDir(),
	}); err != nil {
		c.Logger.Warn("failed to persist path override", "err", err)
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
func (c *Container) SetClaudeDir(newDir string) error {
	info, err := os.Stat(newDir)
	if err != nil {
		return fmt.Errorf("claude projects directory not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", newDir)
	}

	// Hold c.mu so the override write is serialised with ReloadEngramDB (both
	// persist the same file).
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Paths.SetClaudeDir(newDir)
	if err := config.SaveOverrides(c.Config.ConfigHome, config.Overrides{
		EngramDbPath:      c.Paths.EngramDB(),
		ClaudeProjectsDir: newDir,
	}); err != nil {
		c.Logger.Warn("failed to persist path override", "err", err)
	}
	return nil
}

// Close releases database connections.
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
}
