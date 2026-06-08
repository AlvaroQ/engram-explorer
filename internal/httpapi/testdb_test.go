package httpapi_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// seedEngramDB creates a temp SQLite DB with the full engram schema seeded
// from testdata/schema/engram-schema.sql and returns its path.
//
// We build a minimal but correct schema manually rather than parsing the SQL
// dump, which avoids trigger parsing issues when splitting on semicolons. The
// FTS shadow tables are auto-created by SQLite when we create the VIRTUAL TABLEs.
func seedEngramDB(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer db.Close()

	// Build schema statement by statement (no multi-statement exec in database/sql).
	schema := []string{
		`CREATE TABLE sessions (
			id         TEXT PRIMARY KEY,
			project    TEXT NOT NULL,
			directory  TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT (datetime('now')),
			ended_at   TEXT,
			summary    TEXT
		)`,
		`CREATE TABLE observations (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id      TEXT    NOT NULL,
			type            TEXT    NOT NULL,
			title           TEXT    NOT NULL,
			content         TEXT    NOT NULL,
			tool_name       TEXT,
			project         TEXT,
			scope           TEXT    NOT NULL DEFAULT 'project',
			topic_key       TEXT,
			normalized_hash TEXT,
			revision_count  INTEGER NOT NULL DEFAULT 1,
			duplicate_count INTEGER NOT NULL DEFAULT 1,
			last_seen_at    TEXT,
			created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			deleted_at      TEXT,
			sync_id         TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE VIRTUAL TABLE observations_fts USING fts5(
			title, content, tool_name, type, project, topic_key,
			content='observations', content_rowid='id'
		)`,
		`CREATE TABLE user_prompts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			project    TEXT,
			created_at TEXT    NOT NULL DEFAULT (datetime('now')),
			sync_id    TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE VIRTUAL TABLE prompts_fts USING fts5(
			content, project,
			content='user_prompts', content_rowid='id'
		)`,
		`CREATE TABLE sync_enrolled_projects (
			project     TEXT PRIMARY KEY,
			enrolled_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE sync_state (
			target_key           TEXT PRIMARY KEY,
			lifecycle            TEXT NOT NULL DEFAULT 'idle',
			last_enqueued_seq    INTEGER NOT NULL DEFAULT 0,
			last_acked_seq       INTEGER NOT NULL DEFAULT 0,
			last_pulled_seq      INTEGER NOT NULL DEFAULT 0,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			backoff_until        TEXT,
			lease_owner          TEXT,
			lease_until          TEXT,
			last_error           TEXT,
			updated_at           TEXT NOT NULL DEFAULT (datetime('now')),
			reason_code          TEXT,
			reason_message       TEXT
		)`,
		// sync_state must exist before sync_mutations due to FK.
		`CREATE TABLE sync_mutations (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			target_key  TEXT NOT NULL,
			entity      TEXT NOT NULL,
			entity_key  TEXT NOT NULL,
			op          TEXT NOT NULL,
			payload     TEXT NOT NULL,
			source      TEXT NOT NULL DEFAULT 'local',
			occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
			acked_at    TEXT,
			project     TEXT NOT NULL DEFAULT ''
		)`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec %q: %v", truncate(stmt, 60), err)
		}
	}

	// Seed minimal data.
	seedData(t, db)
	return path
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func seedData(t *testing.T, db *sql.DB) {
	t.Helper()
	// Insert a session (required as FK for observations/prompts).
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES ('sess-1', 'myproject', '/tmp', '2026-01-01 10:00:00')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// Insert an observation.
	if _, err := db.Exec(`INSERT INTO observations (id, session_id, type, title, content, project, scope, topic_key, created_at, updated_at) VALUES (1, 'sess-1', 'note', 'Test Obs', 'hello world', 'myproject', 'project', 'test/key', '2026-01-01 10:00:00', '2026-01-01 11:00:00')`); err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	// Insert a prompt.
	if _, err := db.Exec(`INSERT INTO user_prompts (id, session_id, content, project, created_at) VALUES (1, 'sess-1', 'test prompt content', 'myproject', '2026-01-01 10:00:00')`); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}
	// Insert sync_state (needed for overview sync_summary).
	if _, err := db.Exec(`INSERT INTO sync_state (target_key, lifecycle, last_enqueued_seq, last_acked_seq) VALUES ('cloud', 'idle', 0, 0)`); err != nil {
		t.Logf("seed sync_state warning: %v", err)
	}
}
