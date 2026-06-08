package httpapi

import (
	"database/sql"
	"log/slog"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// Container holds all shared dependencies for the HTTP server.
type Container struct {
	Config config.Config
	RoDB   *sql.DB // read-only pool (4 conns)
	RWDB   *sql.DB // read-write pool (1 conn), may be nil if open failed
	Logger *slog.Logger
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
func NewContainer(cfg config.Config, logger *slog.Logger) (*Container, error) {
	roDB, err := sqlite.OpenReadOnly(cfg.EngramDbPath)
	if err != nil {
		return nil, err
	}

	var rwDB *sql.DB
	if cfg.ReadOnly {
		logger.Info("read-only mode enabled (ENGRAM_DASH_READONLY); all write routes return 503")
	} else if rw, rwErr := sqlite.OpenReadWrite(cfg.EngramDbPath); rwErr != nil {
		logger.Warn("read-write database unavailable; write routes will be disabled",
			"path", cfg.EngramDbPath,
			"err", rwErr,
		)
	} else {
		rwDB = rw
	}

	return &Container{
		Config: cfg,
		RoDB:   roDB,
		RWDB:   rwDB,
		Logger: logger,
	}, nil
}

// Close releases database connections.
func (c *Container) Close() {
	if c.RoDB != nil {
		c.RoDB.Close()
	}
	if c.RWDB != nil {
		c.RWDB.Close()
	}
}
