package sqlite

import (
	"context"
	"database/sql"
	"sync"
)

// Querier is the subset of *sql.DB methods used across the services layer.
// Both *sql.DB and *SwapDB satisfy it, which lets the read/write pools be
// hot-swapped (see SwapDB) without changing service call sites.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Begin() (*sql.Tx, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	Prepare(query string) (*sql.Stmt, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	Conn(ctx context.Context) (*sql.Conn, error)
}

var (
	_ Querier = (*sql.DB)(nil)
	_ Querier = (*SwapDB)(nil)
)

// SwapDB wraps a *sql.DB whose underlying pool can be replaced at runtime (when
// the user repoints the dashboard at a different database file). Every Querier
// method forwards to the current pool under a read lock, so in-flight callers
// and a concurrent Swap never race.
type SwapDB struct {
	mu  sync.RWMutex
	cur *sql.DB
}

func NewSwapDB(db *sql.DB) *SwapDB { return &SwapDB{cur: db} }

func (s *SwapDB) Current() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Swap replaces the underlying pool and returns the previous one so the caller
// can close it after a grace period (letting in-flight queries drain).
func (s *SwapDB) Swap(next *sql.DB) (previous *sql.DB) {
	s.mu.Lock()
	previous = s.cur
	s.cur = next
	s.mu.Unlock()
	return previous
}

func (s *SwapDB) Query(q string, a ...any) (*sql.Rows, error) { return s.Current().Query(q, a...) }
func (s *SwapDB) QueryContext(ctx context.Context, q string, a ...any) (*sql.Rows, error) {
	return s.Current().QueryContext(ctx, q, a...)
}
func (s *SwapDB) QueryRow(q string, a ...any) *sql.Row { return s.Current().QueryRow(q, a...) }
func (s *SwapDB) QueryRowContext(ctx context.Context, q string, a ...any) *sql.Row {
	return s.Current().QueryRowContext(ctx, q, a...)
}
func (s *SwapDB) Exec(q string, a ...any) (sql.Result, error) { return s.Current().Exec(q, a...) }
func (s *SwapDB) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	return s.Current().ExecContext(ctx, q, a...)
}
func (s *SwapDB) Begin() (*sql.Tx, error) { return s.Current().Begin() }
func (s *SwapDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.Current().BeginTx(ctx, opts)
}
func (s *SwapDB) Prepare(q string) (*sql.Stmt, error) { return s.Current().Prepare(q) }
func (s *SwapDB) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return s.Current().PrepareContext(ctx, q)
}
func (s *SwapDB) Conn(ctx context.Context) (*sql.Conn, error) { return s.Current().Conn(ctx) }
