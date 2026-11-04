package httpapi

import (
	"context"
	"database/sql"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// Adapter interfaces the host discovers on provider instances (resolved per
// request through Registry.Find; nothing switches on a provider ID).

// sqliteStore is implemented by instances that serve the Engram SQLite pools
// (the engram provider). The host's JSON API and memory pages read through it.
type sqliteStore interface {
	ContainerDeps() (roDB sqlite.Querier, rwDB sqlite.Querier, paths *config.RuntimePaths)
}

// transcriptSource is implemented by instances that serve Claude Code
// transcripts (the cc-sessions provider).
type transcriptSource interface {
	RuntimePaths() *config.RuntimePaths
}

func isSQLiteStore(i providers.Instance) bool { _, ok := i.(sqliteStore); return ok }
func isTranscriptSource(i providers.Instance) bool {
	_, ok := i.(transcriptSource)
	return ok
}

// ---------------------------------------------------------------------------
// Per-request resolution
// ---------------------------------------------------------------------------

// storeHandles returns the pools of the live SQLite store. rw is nil unless
// the instance is Writable. Both are nil when no store is enabled. Legacy
// containers (no registry) return their own pools.
func (c *Container) storeHandles() (ro, rw sqlite.Querier) {
	if c.Registry == nil {
		return c.RoDB, c.RWDB
	}
	inst := c.Registry.Find(isSQLiteStore)
	if inst == nil {
		return nil, nil
	}
	ro, rw, _ = inst.(sqliteStore).ContainerDeps()
	if !instanceWritable(inst) {
		rw = nil
	}
	return ro, rw
}

// instanceWritable reports whether inst implements providers.Writable and
// accepts writes right now.
func instanceWritable(inst providers.Instance) bool {
	w, ok := inst.(providers.Writable)
	return ok && w.Writable()
}

// writable reports whether the write routes may run: the live store is
// Writable (legacy containers: a read-write pool is open).
func (c *Container) writable() bool {
	if c.Registry == nil {
		return c.RWDB != nil
	}
	return instanceWritable(c.Registry.Find(isSQLiteStore))
}

// ---------------------------------------------------------------------------
// liveDB — a Querier that follows the live store
// ---------------------------------------------------------------------------

// liveDB is the RoDB/RWDB of a registry-backed container: every call goes to
// the pool of the store that is live at that moment, so the route handlers
// never keep a pool a toggle or profile switch has closed. With no store (or
// no writable pool for rw) calls fail like a closed database; the route gates
// answer 503 before that can happen.
type liveDB struct {
	c  *Container
	rw bool
}

// unavailableDB is a closed pool: every call fails with "sql: database is closed".
var unavailableDB = func() *sql.DB {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		panic("httpapi: open placeholder pool: " + err.Error())
	}
	_ = db.Close()
	return db
}()

func (l liveDB) db() sqlite.Querier {
	ro, rw := l.c.storeHandles()
	q := ro
	if l.rw {
		q = rw
	}
	if q == nil {
		return unavailableDB
	}
	return q
}

func (l liveDB) Query(q string, args ...any) (*sql.Rows, error) { return l.db().Query(q, args...) }
func (l liveDB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return l.db().QueryContext(ctx, q, args...)
}
func (l liveDB) QueryRow(q string, args ...any) *sql.Row { return l.db().QueryRow(q, args...) }
func (l liveDB) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return l.db().QueryRowContext(ctx, q, args...)
}
func (l liveDB) Exec(q string, args ...any) (sql.Result, error) { return l.db().Exec(q, args...) }
func (l liveDB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return l.db().ExecContext(ctx, q, args...)
}
func (l liveDB) Begin() (*sql.Tx, error) { return l.db().Begin() }
func (l liveDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return l.db().BeginTx(ctx, opts)
}
func (l liveDB) Prepare(q string) (*sql.Stmt, error) { return l.db().Prepare(q) }
func (l liveDB) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return l.db().PrepareContext(ctx, q)
}
func (l liveDB) Conn(ctx context.Context) (*sql.Conn, error) { return l.db().Conn(ctx) }
