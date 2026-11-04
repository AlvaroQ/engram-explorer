package main

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// makeEngramDB writes a minimal Engram database (the tables the provider
// validates) at dir/engram.db and returns its path.
func makeEngramDB(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{"CREATE TABLE observations (id INTEGER)", "CREATE TABLE sessions (id INTEGER)"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	return path
}

// writeProfileConfig persists a config.json whose active profile points the
// Engram provider at dbPath, and returns its bytes.
func writeProfileConfig(t *testing.T, home, dbPath string) []byte {
	t.Helper()
	st := config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: dbPath}}},
		},
		CCAccounts: []config.CCAccount{{ID: "default", Label: "Personal", Path: filepath.Join(home, "cc"), Enabled: true}},
	}
	if err := config.SaveProfileStore(home, &st); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}
	return readFile(t, filepath.Join(home, "config.json"))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func servedEngramPath(t *testing.T, c *httpapi.Container) string {
	t.Helper()
	inst := c.Registry.Instance("engram")
	if inst == nil {
		t.Fatal("engram provider is not enabled")
	}
	return inst.Health(context.Background()).Path
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s must not exist (stat err %v)", filepath.Base(path), err)
	}
}

// TestBuildContainer_DemoServesDemoDBAndWritesNothing covers S1: with a
// config.json profile pointing at a real database, demo mode must serve the
// demo database, and nothing done in demo mode (DB switch, preference write)
// may modify config.json or create explorer-settings.json.
func TestBuildContainer_DemoServesDemoDBAndWritesNothing(t *testing.T) {
	home := t.TempDir()
	realDB := makeEngramDB(t, filepath.Join(t.TempDir(), "real"))
	demoDB := makeEngramDB(t, filepath.Join(t.TempDir(), "demo"))
	before := writeProfileConfig(t, home, realDB)

	cfg := config.Config{ConfigHome: home, EngramDbPath: demoDB, DemoMode: true, DaemonTimeoutMs: 50}
	c := buildContainer(cfg, quietLogger())
	c.CloseGrace = 0
	t.Cleanup(c.Close)

	if got := servedEngramPath(t, c); got != demoDB {
		t.Fatalf("demo mode serves %q, want the demo DB %q", got, demoDB)
	}

	other := makeEngramDB(t, filepath.Join(t.TempDir(), "other"))
	if err := c.ReloadEngramDB(other); err != nil {
		t.Fatalf("ReloadEngramDB: %v", err)
	}
	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpapi.NewServeMux(c).ServeHTTP(httptest.NewRecorder(), req)

	if after := readFile(t, filepath.Join(home, "config.json")); !bytes.Equal(before, after) {
		t.Errorf("demo mode modified config.json:\n%s", after)
	}
	assertNoFile(t, config.OverridesPath(home))
}

// TestBuildContainer_EnvOverridesProfileAndIsNotSaved covers S6: an
// environment variable wins over the config.json profile at boot, and the env
// value is never written to config.json.
func TestBuildContainer_EnvOverridesProfileAndIsNotSaved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_PROJECTS_DIR", "")
	t.Setenv("ENGRAM_DAEMON_URL", "")
	t.Setenv("DASHBOARD_HOST", "")

	dataDir := filepath.Join(home, "env-data")
	envDB := makeEngramDB(t, dataDir)
	profileDB := makeEngramDB(t, filepath.Join(home, "profile"))
	before := writeProfileConfig(t, dataDir, profileDB) // ENGRAM_DATA_DIR is the config home
	t.Setenv("ENGRAM_DATA_DIR", dataDir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := buildContainer(cfg, quietLogger())
	t.Cleanup(c.Close)

	if got := servedEngramPath(t, c); got != envDB {
		t.Fatalf("served %q, want the ENGRAM_DATA_DIR database %q (env > config.json)", got, envDB)
	}
	if after := readFile(t, filepath.Join(dataDir, "config.json")); !bytes.Equal(before, after) {
		t.Errorf("env override was persisted into config.json:\n%s", after)
	}
}

// TestBuildContainer_UnreadableConfigIsNotOverwritten: when config.json cannot
// be decoded the server still boots (env + defaults) on an ephemeral store,
// and later changes never overwrite the file the user may want to repair.
func TestBuildContainer_UnreadableConfigIsNotOverwritten(t *testing.T) {
	home := t.TempDir()
	db := makeEngramDB(t, filepath.Join(t.TempDir(), "db"))
	cfgPath := filepath.Join(home, "config.json")
	corrupt := []byte("{not json")
	if err := os.WriteFile(cfgPath, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}

	c := buildContainer(config.Config{ConfigHome: home, EngramDbPath: db, DaemonTimeoutMs: 50}, quietLogger())
	c.CloseGrace = 0
	t.Cleanup(c.Close)

	if got := servedEngramPath(t, c); got != db {
		t.Fatalf("served %q, want %q", got, db)
	}
	if err := c.ReloadEngramDB(makeEngramDB(t, filepath.Join(t.TempDir(), "other"))); err != nil {
		t.Fatalf("ReloadEngramDB: %v", err)
	}
	if after := readFile(t, cfgPath); !bytes.Equal(corrupt, after) {
		t.Errorf("unreadable config.json was overwritten:\n%s", after)
	}
}
