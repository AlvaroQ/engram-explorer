package httpapi_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// makeEngramDB seeds a sqlite file with the minimal Engram schema the engram
// provider validates (observations + sessions tables) plus a marker table the
// test reads to confirm which database is live.
func makeEngramDB(t *testing.T, v string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, stmt := range []string{
		"CREATE TABLE observations (id INTEGER)",
		"CREATE TABLE sessions (id INTEGER)",
		"CREATE TABLE marker (v TEXT)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if _, err := db.Exec("INSERT INTO marker VALUES (?)", v); err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Close()
	return path
}

// TestReloadEngramDB_RegistryPath reproduces the registry-backed boot path (the
// one the real binary uses) and asserts that ReloadEngramDB hot-swaps the live
// pool without panicking. Before the fix, c.roSwap was nil on this path so the
// swap dereferenced a nil pointer — the "DB-path switcher does nothing/crashes".
func TestReloadEngramDB_RegistryPath(t *testing.T) {
	pathA := makeEngramDB(t, "a")
	pathB := makeEngramDB(t, "b")

	cfg := config.Config{
		EngramDbPath:  pathA,
		EngramDataDir: filepath.Dir(pathA),
		ConfigHome:    t.TempDir(),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	prof := config.Profile{Providers: map[string]config.ProviderConfig{
		"engram": {Enabled: true, Path: pathA},
	}}
	reg.Boot(context.Background(), httpapi.AdaptProfile(prof))

	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
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
	// This must not panic (regression guard for the nil c.roSwap bug).
	if err := c.ReloadEngramDB(pathB); err != nil {
		t.Fatalf("ReloadEngramDB on registry path: %v", err)
	}
	if got := read(); got != "b" {
		t.Fatalf("after reload: got %q, want b", got)
	}
}

// TestReloadEngramDB_HotSwap verifies that repointing the database swaps the
// live read pool, saves the path into the active profile of config.json (never
// into the legacy explorer-settings.json), and that an invalid path is
// rejected without disturbing the current connection.
func TestReloadEngramDB_HotSwap(t *testing.T) {
	pathA := makeEngramDB(t, "a")
	pathB := makeEngramDB(t, "b")

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
	c.ProfileStore = config.NewStore(cfg.ConfigHome, config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: pathA}}},
		},
	})
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

	// The path was persisted into the active profile in config.json only.
	saved, err := config.LoadProfileStore(cfg.ConfigHome)
	if err != nil {
		t.Fatalf("LoadProfileStore: %v", err)
	}
	if got := saved.Profiles["default"].ProviderCfg("engram").Path; got != pathB {
		t.Errorf("persisted engram path: got %q, want %q", got, pathB)
	}
	if _, err := os.Stat(config.OverridesPath(cfg.ConfigHome)); !os.IsNotExist(err) {
		t.Errorf("explorer-settings.json must not be written (stat err %v)", err)
	}
}
