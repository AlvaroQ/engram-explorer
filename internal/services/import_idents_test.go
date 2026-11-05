package services_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// TestImportMerge_RejectsUnsafeColumnNames covers the path where a required
// table is missing locally, so the source columns are not compared with a
// local schema. A crafted column name must be rejected before it can reach an
// INSERT statement, and the local database must stay untouched.
func TestImportMerge_RejectsUnsafeColumnNames(t *testing.T) {
	db, path := newWriteDB(t)
	if _, err := db.Exec(`DROP TABLE user_prompts`); err != nil {
		t.Fatalf("drop local user_prompts: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('keep', 'p', '/tmp')`); err != nil {
		t.Fatalf("seed local session: %v", err)
	}

	srcPath := filepath.Join(filepath.Dir(path), "evil.db")
	src, err := sql.Open("sqlite", "file:"+srcPath)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	for _, s := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, project TEXT, directory TEXT, started_at TEXT, ended_at TEXT, summary TEXT)`,
		`CREATE TABLE observations (id INTEGER PRIMARY KEY, session_id TEXT)`,
		`CREATE TABLE user_prompts (id INTEGER PRIMARY KEY, "content) VALUES (1); DROP TABLE sessions; --" TEXT, sync_id TEXT)`,
	} {
		if _, err := src.Exec(s); err != nil {
			t.Fatalf("src setup %q: %v", s, err)
		}
	}
	src.Close()

	// Align the source sessions/observations schema with the local one so only
	// the user_prompts column name is under test.
	alignTable(t, db, srcPath, "sessions")
	alignTable(t, db, srcPath, "observations")

	_, err = services.ImportMerge(context.Background(), db, srcPath, filepath.Dir(path))
	var die *services.DatabaseImportError
	if !errors.As(err, &die) {
		t.Fatalf("expected *DatabaseImportError, got %T: %v", err, err)
	}
	if die.Code != services.DatabaseImportCodeSchemaMismatch {
		t.Errorf("code: got %q, want %q", die.Code, services.DatabaseImportCodeSchemaMismatch)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'keep'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("local sessions changed: count=%d err=%v", n, err)
	}
}

// alignTable recreates table in the database at srcPath with the exact local
// CREATE statement, so validation sees matching columns.
func alignTable(t *testing.T, local *sql.DB, srcPath, table string) {
	t.Helper()
	var ddl string
	if err := local.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&ddl); err != nil {
		t.Fatalf("local ddl %s: %v", table, err)
	}
	src, err := sql.Open("sqlite", "file:"+srcPath)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer src.Close()
	if _, err := src.Exec(`DROP TABLE ` + table); err != nil {
		t.Fatalf("drop src %s: %v", table, err)
	}
	if _, err := src.Exec(ddl); err != nil {
		t.Fatalf("create src %s: %v", table, err)
	}
}
