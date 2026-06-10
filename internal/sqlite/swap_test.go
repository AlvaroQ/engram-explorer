package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// makeMarkerDB seeds a read-only sqlite pool whose marker table holds v.
func makeMarkerDB(t *testing.T, v int) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "marker.db")
	seed, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if _, err := seed.Exec("CREATE TABLE marker (v INTEGER)"); err != nil {
		t.Fatalf("seed create: %v", err)
	}
	if _, err := seed.Exec("INSERT INTO marker VALUES (?)", v); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	seed.Close()

	db, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func readMarker(t *testing.T, q sqlite.Querier) int {
	t.Helper()
	var v int
	if err := q.QueryRow("SELECT v FROM marker").Scan(&v); err != nil {
		t.Fatalf("query marker: %v", err)
	}
	return v
}

// TestSwapDB_SwapsUnderlyingPool verifies that a SwapDB forwards to whichever
// pool is current, and that Swap returns the previous pool.
func TestSwapDB_SwapsUnderlyingPool(t *testing.T) {
	a := makeMarkerDB(t, 1)
	b := makeMarkerDB(t, 2)

	s := sqlite.NewSwapDB(a)
	if got := readMarker(t, s); got != 1 {
		t.Fatalf("before swap: got %d, want 1", got)
	}

	if prev := s.Swap(b); prev != a {
		t.Fatal("Swap should return the previous pool")
	}
	if got := readMarker(t, s); got != 2 {
		t.Fatalf("after swap: got %d, want 2", got)
	}
	if s.Current() != b {
		t.Fatal("Current should return the swapped-in pool")
	}
}
