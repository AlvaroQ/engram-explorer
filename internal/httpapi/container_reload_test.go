package httpapi_test

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// makeReloadDB seeds a sqlite file whose marker table holds v.
func makeReloadDB(t *testing.T, v string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE marker (v TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec("INSERT INTO marker VALUES (?)", v); err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Close()
	return path
}

// TestReloadEngramDB_HotSwap verifies that repointing the database swaps the
// live read pool, persists the override, and that an invalid path is rejected
// without disturbing the current connection.
func TestReloadEngramDB_HotSwap(t *testing.T) {
	pathA := makeReloadDB(t, "a")
	pathB := makeReloadDB(t, "b")

	cfg := config.Config{
		EngramDbPath:  pathA,
		EngramDataDir: filepath.Dir(pathA),
		ConfigHome:    t.TempDir(),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	c.CloseGrace = 0 // close superseded pools synchronously so temp files unlock
	t.Cleanup(c.Close)

	read := func() string {
		var v string
		if err := c.RoDB.QueryRow("SELECT v FROM marker").Scan(&v); err != nil {
			t.Fatalf("read: %v", err)
		}
		return v
	}

	if got := read(); got != "a" {
		t.Fatalf("initial read: got %q, want a", got)
	}

	if err := c.ReloadEngramDB(pathB); err != nil {
		t.Fatalf("ReloadEngramDB: %v", err)
	}
	if got := read(); got != "b" {
		t.Fatalf("after reload: got %q, want b", got)
	}
	if c.Paths.EngramDB() != pathB {
		t.Errorf("Paths.EngramDB: got %q, want %q", c.Paths.EngramDB(), pathB)
	}

	// An invalid path must fail and leave the current DB intact.
	if err := c.ReloadEngramDB(filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Fatal("expected error reloading a missing path")
	}
	if got := read(); got != "b" {
		t.Fatalf("after failed reload: got %q, want b (unchanged)", got)
	}

	// The override was persisted under the config home.
	if ov := config.LoadOverrides(cfg.ConfigHome); ov.EngramDbPath != pathB {
		t.Errorf("persisted override: got %q, want %q", ov.EngramDbPath, pathB)
	}
}
