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
// The read-only pool is required; a failure is fatal.
// The read-write pool is optional; a failure is logged but the server keeps running.
func NewContainer(cfg config.Config, logger *slog.Logger) (*Container, error) {
	roDB, err := sqlite.OpenReadOnly(cfg.EngramDbPath)
	if err != nil {
		return nil, err
	}

	rwDB, err := sqlite.OpenReadWrite(cfg.EngramDbPath)
	if err != nil {
		logger.Warn("read-write database unavailable; write routes will be disabled",
			"path", cfg.EngramDbPath,
			"err", err,
		)
		rwDB = nil
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
