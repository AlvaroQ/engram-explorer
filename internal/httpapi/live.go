package httpapi

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
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

func isSQLiteStore(i providers.Instance) bool { _, ok := i.(sqliteStore); return ok }

// ---------------------------------------------------------------------------
// Per-request resolution
// ---------------------------------------------------------------------------

// leaseKey is the request-context key of the providers.Lease of a request.
type leaseKey struct{}

// withLeases pins, for every request, the provider instances that are Enabled
// when it starts (providers.Registry.Acquire) and releases them when the
// handler returns. Every per-request resolution below reads the pinned
// instances, so a toggle or profile switch mid-request neither hands the
// request a closed pool nor swaps its module out from under it: the registry
// closes a superseded instance only after its leases are released.
func (c *Container) withLeases(next http.Handler) http.Handler {
	if c.Registry == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := c.Registry.Acquire()
		defer l.Release()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), leaseKey{}, l)))
	})
}

// leaseFrom returns the lease of r, or nil when r is nil or was not served
// through withLeases.
func leaseFrom(r *http.Request) *providers.Lease {
	if r == nil {
		return nil
	}
	l, _ := r.Context().Value(leaseKey{}).(*providers.Lease)
	return l
}

// find resolves the instance matching a capability for r: from the lease of
// r when it has one, otherwise from the registry as it is right now.
func (c *Container) find(r *http.Request, match func(providers.Instance) bool) providers.Instance {
	if l := leaseFrom(r); l != nil {
		return l.Find(match)
	}
	return c.Registry.Find(match)
}

// findAll resolves every instance matching a capability for r: from the
// lease of r when it has one, otherwise from the registry as it is now.
func (c *Container) findAll(r *http.Request, match func(providers.Instance) bool) []providers.Instance {
	if l := leaseFrom(r); l != nil {
		return l.FindAll(match)
	}
	return c.Registry.FindAll(match)
}

func isAgentSessionSource(i providers.Instance) bool {
	_, ok := i.(providers.AgentSessionSource)
	return ok
}

// agentSessionSources returns the sources of every live AgentSessionSource
// for r, in registry order (nil for legacy containers).
func (c *Container) agentSessionSources(r *http.Request) []agentsessions.Source {
	if c.Registry == nil {
		return nil
	}
	var out []agentsessions.Source
	for _, inst := range c.findAll(r, isAgentSessionSource) {
		out = append(out, inst.(providers.AgentSessionSource).AgentSessionSources()...)
	}
	return out
}

// storeHandles returns the pools of the SQLite store serving r (nil r: the
// store live right now). rw is nil unless the instance is Writable. Both are
// nil when no store is enabled. Legacy containers (no registry) return their
// own pools.
func (c *Container) storeHandles(r *http.Request) (ro, rw sqlite.Querier) {
	if c.Registry == nil {
		return c.RoDB, c.RWDB
	}
	inst := c.find(r, isSQLiteStore)
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

// writable reports whether the write routes may run for r: the store serving
// it is Writable (legacy containers: a read-write pool is open).
func (c *Container) writable(r *http.Request) bool {
	if c.Registry == nil {
		return c.RWDB != nil
	}
	return instanceWritable(c.find(r, isSQLiteStore))
}

// ro returns the read-only pool serving r. The route gates answer 503 before
// a handler runs without a store; should one still run, its calls fail like a
// closed database instead of panicking.
func (c *Container) ro(r *http.Request) sqlite.Querier {
	if c.Registry == nil {
		return c.RoDB
	}
	if ro, _ := c.storeHandles(r); ro != nil {
		return ro
	}
	return unavailableDB
}

// rw returns the read-write pool serving r (see ro); handlers check
// guardRWDB first.
func (c *Container) rw(r *http.Request) sqlite.Querier {
	if c.Registry == nil {
		return c.RWDB
	}
	if _, rw := c.storeHandles(r); rw != nil {
		return rw
	}
	return unavailableDB
}

// ---------------------------------------------------------------------------
// liveDB — a Querier that follows the live store
// ---------------------------------------------------------------------------

// liveDB is the RoDB/RWDB of a registry-backed container: every call goes to
// the pool of the store that is live at that moment. Route handlers do not use
// it — they read the pools pinned for their request (c.ro / c.rw) — it serves
// callers outside a request. With no store (or no writable pool for rw) calls
// fail like a closed database.
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
	ro, rw := l.c.storeHandles(nil)
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
