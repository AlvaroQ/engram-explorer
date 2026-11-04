package httpapi_test

import (
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

// internalErrorFragments are substrings of raw OS / SQLite error text that must
// never reach the user-facing settings page (S5 + S8: no err.Error() internals).
var internalErrorFragments = []string{
	"no such file or directory",
	"stat ",
	"open ",
	"file is not a database",
	"SQLITE",
	"sqlite:",
	"readdirent",
	"is a directory",
}

// makeClaudeDir returns a temp directory with one project subfolder, the shape
// the cc-sessions provider accepts.
func makeClaudeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "-Users-me-project"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	return dir
}

// makeSchemalessDB seeds a valid SQLite file without the Engram tables.
func makeSchemalessDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "other.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE marker (v TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	db.Close()
	return path
}

// settingsFixture builds a legacy container on Engram DB "a" and a valid Claude
// dir, persists both as the current overrides, and returns the served handler.
type settingsFixture struct {
	c         *httpapi.Container
	h         http.Handler
	cfg       config.Config
	dbPath    string
	claudeDir string
}

func newSettingsFixture(t *testing.T) settingsFixture {
	t.Helper()
	dbPath := makeEngramDB(t, "a")
	claudeDir := makeClaudeDir(t)
	cfg := config.Config{
		EngramDbPath:      dbPath,
		EngramDataDir:     filepath.Dir(dbPath),
		ClaudeProjectsDir: claudeDir,
		ConfigHome:        t.TempDir(),
	}
	if err := config.SaveOverrides(cfg.ConfigHome, config.Overrides{
		EngramDbPath:      dbPath,
		ClaudeProjectsDir: claudeDir,
	}); err != nil {
		t.Fatalf("seed overrides: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	c.CloseGrace = 0
	t.Cleanup(c.Close)
	return settingsFixture{c: c, h: httpapi.NewServeMux(c), cfg: cfg, dbPath: dbPath, claudeDir: claudeDir}
}

func (f settingsFixture) post(t *testing.T, route, value string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"path": {value}}
	req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	return w
}

func (f settingsFixture) marker(t *testing.T) string {
	t.Helper()
	var v string
	if err := f.c.RoDB.QueryRow("SELECT v FROM marker").Scan(&v); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return v
}

func assertNoInternals(t *testing.T, body string) {
	t.Helper()
	for _, frag := range internalErrorFragments {
		if strings.Contains(body, frag) {
			t.Errorf("response leaks internal error text %q", frag)
		}
	}
}

// TestEngramDBPost_RejectsInvalidDatabase covers S5: the Engram DB path must be
// a regular SQLite file with the Engram schema before the dashboard repoints.
// Every rejection leaves the live DB, the runtime path and the persisted
// overrides unchanged and shows a user-safe message.
func TestEngramDBPost_RejectsInvalidDatabase(t *testing.T) {
	notSQLite := filepath.Join(t.TempDir(), "notes.db")
	if err := os.WriteFile(notSQLite, []byte("just some text, not a database\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cases := []struct {
		name    string
		path    string
		wantMsg string
	}{
		{"directory", t.TempDir(), "not a regular file"},
		{"missing file", filepath.Join(t.TempDir(), "missing.db"), "database file not found"},
		{"non-SQLite file", notSQLite, "not an SQLite database"},
		{"SQLite without Engram tables", makeSchemalessDB(t), "not an Engram database"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSettingsFixture(t)

			w := f.post(t, "/settings/engram-db", tc.path)
			if w.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (settings re-render with error)", w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.wantMsg) {
				t.Errorf("body does not contain user message %q", tc.wantMsg)
			}
			assertNoInternals(t, body)

			if got := f.marker(t); got != "a" {
				t.Errorf("live DB changed: marker %q, want a", got)
			}
			if got := f.c.Paths.EngramDB(); got != f.dbPath {
				t.Errorf("runtime path changed: got %q, want %q", got, f.dbPath)
			}
			ov := config.LoadOverrides(f.cfg.ConfigHome)
			if ov.EngramDbPath != f.dbPath || ov.ClaudeProjectsDir != f.claudeDir {
				t.Errorf("persisted overrides changed: got %+v", ov)
			}
		})
	}
}

// TestEngramDBPost_ValidDatabaseSwitches keeps the happy path: a valid Engram
// DB is swapped in live and persisted.
func TestEngramDBPost_ValidDatabaseSwitches(t *testing.T) {
	f := newSettingsFixture(t)
	pathB := makeEngramDB(t, "b")

	w := f.post(t, "/settings/engram-db", pathB)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body: %s", w.Code, w.Body.String())
	}
	if got := f.marker(t); got != "b" {
		t.Errorf("live DB: marker %q, want b", got)
	}
	if ov := config.LoadOverrides(f.cfg.ConfigHome); ov.EngramDbPath != pathB {
		t.Errorf("persisted EngramDbPath: got %q, want %q", ov.EngramDbPath, pathB)
	}
}

// TestClaudeDirPost_RejectsInvalidDirectory covers S5 for the Claude projects
// directory: it must pass the cc-sessions provider validation (an existing
// directory with at least one project folder). Rejections leave the runtime
// path and the persisted overrides unchanged.
func TestClaudeDirPost_RejectsInvalidDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cases := []struct {
		name    string
		path    string
		wantMsg string
	}{
		{"file", file, "not a directory"},
		{"missing path", filepath.Join(t.TempDir(), "missing"), "projects directory not found"},
		{"empty directory", t.TempDir(), "no project folders found"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSettingsFixture(t)

			w := f.post(t, "/settings/claude-dir", tc.path)
			if w.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (settings re-render with error)", w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.wantMsg) {
				t.Errorf("body does not contain user message %q", tc.wantMsg)
			}
			assertNoInternals(t, body)

			if got := f.c.Paths.ClaudeDir(); got != f.claudeDir {
				t.Errorf("runtime Claude dir changed: got %q, want %q", got, f.claudeDir)
			}
			ov := config.LoadOverrides(f.cfg.ConfigHome)
			if ov.EngramDbPath != f.dbPath || ov.ClaudeProjectsDir != f.claudeDir {
				t.Errorf("persisted overrides changed: got %+v", ov)
			}
		})
	}
}

// TestClaudeDirPost_ValidDirectorySwitches keeps the happy path.
func TestClaudeDirPost_ValidDirectorySwitches(t *testing.T) {
	f := newSettingsFixture(t)
	dirB := makeClaudeDir(t)

	w := f.post(t, "/settings/claude-dir", dirB)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body: %s", w.Code, w.Body.String())
	}
	if got := f.c.Paths.ClaudeDir(); got != dirB {
		t.Errorf("runtime Claude dir: got %q, want %q", got, dirB)
	}
	if ov := config.LoadOverrides(f.cfg.ConfigHome); ov.ClaudeProjectsDir != dirB {
		t.Errorf("persisted ClaudeProjectsDir: got %q, want %q", ov.ClaudeProjectsDir, dirB)
	}
}
