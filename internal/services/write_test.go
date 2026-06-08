package services_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ---------------------------------------------------------------------------
// Test DB helpers
// ---------------------------------------------------------------------------

func newWriteDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	dsn := fmt.Sprintf(
		"file:%s?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open write db: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		t.Fatalf("ping write db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, s := range writeTestSchema {
		if _, err := db.Exec(s); err != nil {
			n := len(s)
			if n > 60 {
				n = 60
			}
			t.Fatalf("schema stmt: %v (%s...)", err, s[:n])
		}
	}
	return db, path
}

var writeTestSchema = []string{
	`CREATE TABLE sessions (
		id         TEXT PRIMARY KEY,
		project    TEXT,
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
		sync_id         TEXT
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
		sync_id    TEXT
	)`,
	`CREATE VIRTUAL TABLE prompts_fts USING fts5(
		content, project,
		content='user_prompts', content_rowid='id'
	)`,
	`CREATE TABLE sync_enrolled_projects (
		project     TEXT PRIMARY KEY,
		enrolled_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
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

func seedObs(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO sessions (id, project, directory) VALUES ('s1', 'proj', '/tmp')`,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	res, err := db.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, scope, created_at, updated_at)
		 VALUES ('s1', 'note', 'Test Title', 'test content', 'proj', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`,
	)
	if err != nil {
		t.Fatalf("seed obs: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ---------------------------------------------------------------------------
// Shared helpers tests
// ---------------------------------------------------------------------------

func TestNowSqlite_Format(t *testing.T) {
	now := services.NowSqlite()
	if len(now) != 19 {
		t.Errorf("NowSqlite length: got %d, want 19 (%q)", len(now), now)
	}
	if now[4] != '-' || now[7] != '-' || now[10] != ' ' || now[13] != ':' || now[16] != ':' {
		t.Errorf("NowSqlite format wrong: %q (want YYYY-MM-DD HH:MM:SS)", now)
	}
}

func TestGenSyncID_Format(t *testing.T) {
	for _, prefix := range []string{"obs", "prompt"} {
		id := services.GenSyncID(prefix)
		pfx := prefix + "-"
		if !strings.HasPrefix(id, pfx) {
			t.Errorf("GenSyncID(%q) = %q, want prefix %q", prefix, id, pfx)
		}
		hexPart := strings.TrimPrefix(id, pfx)
		if len(hexPart) != 16 {
			t.Errorf("GenSyncID hex part length: got %d, want 16", len(hexPart))
		}
		for _, c := range hexPart {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("non-hex char %q in GenSyncID result %q", c, id)
				break
			}
		}
	}
}

func TestIsEnrolled(t *testing.T) {
	db, _ := newWriteDB(t)
	if services.IsEnrolled(db, "") {
		t.Error("empty project should not be enrolled")
	}
	if services.IsEnrolled(db, "unknown") {
		t.Error("unknown project should not be enrolled")
	}
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project) VALUES ('myprojx')`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if !services.IsEnrolled(db, "myprojx") {
		t.Error("myprojx should be enrolled after insert")
	}
}

// ---------------------------------------------------------------------------
// Assignment tests
// ---------------------------------------------------------------------------

func TestAssignProject_ObsNotFound(t *testing.T) {
	db, _ := newWriteDB(t)
	_, err := services.AssignProject(context.Background(), db, services.EntityKindObservation, int64(9999), "proj")
	assertWriteErrorCode(t, err, "NOT_FOUND")
}

func TestAssignProject_ObsSuccess_NoEnroll(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)

	result, err := services.AssignProject(context.Background(), db, services.EntityKindObservation, id, "newproj")
	if err != nil {
		t.Fatalf("AssignProject: %v", err)
	}
	if result.Project != "newproj" {
		t.Errorf("project: got %q, want newproj", result.Project)
	}
	if result.MutationSeq != nil {
		t.Errorf("mutation_seq: expected nil (not enrolled), got %v", result.MutationSeq)
	}
	if result.Enrolled {
		t.Error("enrolled: expected false")
	}
}

func TestAssignProject_ObsSuccess_WithEnroll_SyncMutation(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project) VALUES ('synced_proj')`); err != nil {
		t.Fatalf("enroll: %v", err)
	}

	result, err := services.AssignProject(context.Background(), db, services.EntityKindObservation, id, "synced_proj")
	if err != nil {
		t.Fatalf("AssignProject: %v", err)
	}
	if !result.Enrolled {
		t.Error("enrolled: expected true")
	}
	if result.MutationSeq == nil {
		t.Fatal("mutation_seq: expected non-nil when enrolled")
	}

	// Verify sync_mutations row.
	var targetKey, entity, syncID, op, proj string
	if err := db.QueryRow(
		`SELECT target_key, entity, entity_key, op, project FROM sync_mutations WHERE seq = ?`,
		*result.MutationSeq,
	).Scan(&targetKey, &entity, &syncID, &op, &proj); err != nil {
		t.Fatalf("query sync_mutations: %v", err)
	}
	if targetKey != "cloud" {
		t.Errorf("target_key: got %q, want cloud", targetKey)
	}
	if entity != "observation" {
		t.Errorf("entity: got %q, want observation", entity)
	}
	if op != "upsert" {
		t.Errorf("op: got %q, want upsert", op)
	}
	if proj != "synced_proj" {
		t.Errorf("project: got %q, want synced_proj", proj)
	}
	// Verify sync_id format: "obs-<16hex>"
	if !strings.HasPrefix(syncID, "obs-") || len(syncID) != 20 {
		t.Errorf("sync_id format: got %q (want obs-<16hex>)", syncID)
	}
	// Verify nowSqlite timestamp format in occurred_at.
	var occurredAt string
	db.QueryRow(`SELECT occurred_at FROM sync_mutations WHERE seq = ?`, *result.MutationSeq).Scan(&occurredAt)
	if len(occurredAt) != 19 {
		t.Errorf("occurred_at format wrong: %q (want YYYY-MM-DD HH:MM:SS)", occurredAt)
	}
}

// ---------------------------------------------------------------------------
// Observation update tests
// ---------------------------------------------------------------------------

func TestUpdateObservation_NotFound(t *testing.T) {
	db, _ := newWriteDB(t)
	_, err := services.UpdateObservation(context.Background(), db, 9999, services.ObservationPatch{})
	assertWriteErrorCode(t, err, "NOT_FOUND")
}

func TestUpdateObservation_SetType(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)

	result, err := services.UpdateObservation(context.Background(), db, id, services.ObservationPatch{
		TypeProvided: true, Type: "decision",
	})
	if err != nil {
		t.Fatalf("UpdateObservation: %v", err)
	}
	if result.Updated["type"] != "decision" {
		t.Errorf("updated.type: got %v, want decision", result.Updated["type"])
	}
	var typ string
	db.QueryRow(`SELECT type FROM observations WHERE id = ?`, id).Scan(&typ)
	if typ != "decision" {
		t.Errorf("DB type: got %q, want decision", typ)
	}
}

func TestUpdateObservation_OmitField_NotInUpdated(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)

	result, err := services.UpdateObservation(context.Background(), db, id, services.ObservationPatch{
		TypeProvided: true, Type: "note",
	})
	if err != nil {
		t.Fatalf("UpdateObservation: %v", err)
	}
	if _, ok := result.Updated["title"]; ok {
		t.Error("title should NOT appear in Updated when not provided")
	}
	if _, ok := result.Updated["topic_key"]; ok {
		t.Error("topic_key should NOT appear in Updated when not provided")
	}
}

func TestUpdateObservation_EmptyPatch_NoOp(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)

	result, err := services.UpdateObservation(context.Background(), db, id, services.ObservationPatch{})
	if err != nil {
		t.Fatalf("UpdateObservation: %v", err)
	}
	if len(result.Updated) != 0 {
		t.Errorf("Updated should be empty for no-op patch, got %v", result.Updated)
	}
}

// ---------------------------------------------------------------------------
// Deletion tests
// ---------------------------------------------------------------------------

func TestDeleteObservation_SoftDelete(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)

	result, err := services.DeleteEntity(context.Background(), db, services.EntityKindObservation, id)
	if err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if result.Mode != "soft" {
		t.Errorf("mode: got %q, want soft", result.Mode)
	}

	var deletedAt *string
	db.QueryRow(`SELECT deleted_at FROM observations WHERE id = ?`, id).Scan(&deletedAt)
	if deletedAt == nil {
		t.Error("deleted_at should be set after soft delete")
	}
}

func TestDeleteObservation_AlreadyDeleted(t *testing.T) {
	db, _ := newWriteDB(t)
	id := seedObs(t, db)

	if _, err := services.DeleteEntity(context.Background(), db, services.EntityKindObservation, id); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	_, err := services.DeleteEntity(context.Background(), db, services.EntityKindObservation, id)
	assertWriteErrorCode(t, err, "ALREADY_DELETED")
}

func TestDeleteSession_HAS_PROMPTS(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('sp1', 'proj', '/tmp')`)
	db.Exec(`INSERT INTO user_prompts (session_id, content) VALUES ('sp1', 'some prompt')`)

	_, err := services.DeleteEntity(context.Background(), db, services.EntityKindSession, "sp1")
	assertWriteErrorCode(t, err, "HAS_PROMPTS")
}

func TestDeleteSession_HAS_OBSERVATIONS(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('so1', 'proj', '/tmp')`)
	db.Exec(`INSERT INTO observations (session_id, type, title, content, scope, created_at, updated_at) VALUES ('so1', 'note', 'T', 'C', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`)

	_, err := services.DeleteEntity(context.Background(), db, services.EntityKindSession, "so1")
	assertWriteErrorCode(t, err, "HAS_OBSERVATIONS")
}

func TestDeleteSession_Success(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('sc1', 'proj', '/tmp')`)

	result, err := services.DeleteEntity(context.Background(), db, services.EntityKindSession, "sc1")
	if err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if result.Mode != "hard" {
		t.Errorf("mode: got %q, want hard", result.Mode)
	}
}

// ---------------------------------------------------------------------------
// Project rename tests
// ---------------------------------------------------------------------------

func TestRenameProject_SAME_NAME(t *testing.T) {
	db, _ := newWriteDB(t)
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "x", Target: "x", Mode: "rename",
	})
	assertWriteErrorCode(t, err, "SAME_NAME")
}

func TestRenameProject_SOURCE_NOT_FOUND(t *testing.T) {
	db, _ := newWriteDB(t)
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "nosuch", Target: "other", Mode: "rename",
	})
	assertWriteErrorCode(t, err, "SOURCE_NOT_FOUND")
}

func TestRenameProject_ENROLLED_SOURCE_UNSUPPORTED(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r1', 'enrolled_src', '/tmp')`)
	db.Exec(`INSERT INTO sync_enrolled_projects (project) VALUES ('enrolled_src')`)
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "enrolled_src", Target: "other", Mode: "rename",
	})
	assertWriteErrorCode(t, err, "ENROLLED_SOURCE_UNSUPPORTED")
}

func TestRenameProject_TARGET_EXISTS(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r2', 'src', '/tmp')`)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r3', 'tgt', '/tmp')`)
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "src", Target: "tgt", Mode: "rename",
	})
	assertWriteErrorCode(t, err, "TARGET_EXISTS")
}

func TestRenameProject_TARGET_NOT_FOUND_merge(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r4', 'src', '/tmp')`)
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "src", Target: "nosuchmerge", Mode: "merge",
	})
	assertWriteErrorCode(t, err, "TARGET_NOT_FOUND")
}

func TestRenameProject_ENROLLED_TARGET_UNSUPPORTED(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r5', 'src', '/tmp')`)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r6', 'etgt', '/tmp')`)
	db.Exec(`INSERT INTO sync_enrolled_projects (project) VALUES ('etgt')`)
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "src", Target: "etgt", Mode: "merge",
	})
	assertWriteErrorCode(t, err, "ENROLLED_TARGET_UNSUPPORTED")
}

func TestRenameProject_Success(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('r7', 'alpha', '/tmp')`)
	db.Exec(`INSERT INTO observations (session_id, type, title, content, project, scope, created_at, updated_at) VALUES ('r7', 'note', 'T', 'C', 'alpha', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`)

	result, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "alpha", Target: "beta", Mode: "rename",
	})
	if err != nil {
		t.Fatalf("RenameProject: %v", err)
	}
	if result.Affected.Sessions != 1 {
		t.Errorf("sessions: got %d, want 1", result.Affected.Sessions)
	}
	if result.Affected.Observations != 1 {
		t.Errorf("observations: got %d, want 1", result.Affected.Observations)
	}
	var proj string
	db.QueryRow(`SELECT project FROM sessions WHERE id = 'r7'`).Scan(&proj)
	if proj != "beta" {
		t.Errorf("session project: got %q, want beta", proj)
	}
}

func TestRenameProject_ValidationOrder_SameNameBeforeNotFound(t *testing.T) {
	db, _ := newWriteDB(t)
	// source doesn't exist AND source == target → SAME_NAME should win.
	_, err := services.RenameProject(context.Background(), db, services.RenameProjectParams{
		Source: "x", Target: "x", Mode: "rename",
	})
	assertWriteErrorCode(t, err, "SAME_NAME")
}

// ---------------------------------------------------------------------------
// Database export/import tests
// ---------------------------------------------------------------------------

func TestExportSnapshot_WritesValidSQLite(t *testing.T) {
	db, _ := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('ex1', 'proj', '/tmp')`)

	exportPath, err := services.ExportSnapshot(context.Background(), db)
	if err != nil {
		t.Fatalf("ExportSnapshot: %v", err)
	}
	t.Cleanup(func() { os.Remove(exportPath) })

	// Verify it opens as a valid SQLite DB.
	db2, err := sql.Open("sqlite", "file:"+exportPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open exported db: %v", err)
	}
	defer db2.Close()
	var n int
	if err := db2.QueryRow("SELECT 1").Scan(&n); err != nil {
		t.Fatalf("ping exported db: %v", err)
	}
	if n != 1 {
		t.Errorf("ping returned %d, want 1", n)
	}
}

func TestImportMerge_RoundTrip(t *testing.T) {
	db, path := newWriteDB(t)
	db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('imp1', 'projA', '/tmp')`)
	db.Exec(`INSERT INTO observations (session_id, type, title, content, project, scope, created_at, updated_at) VALUES ('imp1', 'note', 'ImportTest', 'content', 'projA', 'project', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`)

	exportPath, err := services.ExportSnapshot(context.Background(), db)
	if err != nil {
		t.Fatalf("ExportSnapshot: %v", err)
	}
	t.Cleanup(func() { os.Remove(exportPath) })

	dataDir := filepath.Dir(path)
	result, err := services.ImportMerge(context.Background(), db, exportPath, dataDir)
	if err != nil {
		t.Fatalf("ImportMerge: %v", err)
	}
	t.Cleanup(func() { os.Remove(result.BackupPath) })

	if result.BackupPath == "" {
		t.Error("backupPath should be non-empty")
	}
	// Rows already exist → imported rows are de-duplicated (skipped or inserted 0).
	// Since sessions have a text PK with INSERT OR IGNORE, they're all skipped.
	if result.Inserted.Sessions != 0 {
		t.Errorf("inserted.sessions: got %d, want 0 (rows already exist)", result.Inserted.Sessions)
	}
}

func TestImportMerge_BadSQLite(t *testing.T) {
	db, path := newWriteDB(t)
	fakePath := filepath.Join(filepath.Dir(path), "notdb.db")
	if err := os.WriteFile(fakePath, []byte("this is not sqlite"), 0o600); err != nil {
		t.Fatalf("write fake file: %v", err)
	}
	t.Cleanup(func() { os.Remove(fakePath) })

	_, err := services.ImportMerge(context.Background(), db, fakePath, filepath.Dir(path))
	if err == nil {
		t.Fatal("expected DatabaseImportError")
	}
	die, ok := err.(*services.DatabaseImportError)
	if !ok {
		t.Fatalf("expected *DatabaseImportError, got %T: %v", err, err)
	}
	if die.Code != services.DatabaseImportCodeBadSQLite {
		t.Errorf("code: got %q, want BAD_SQLITE", die.Code)
	}
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

func assertWriteErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected WriteError %q, got nil", code)
	}
	we, ok := err.(*services.WriteError)
	if !ok {
		t.Fatalf("expected *WriteError, got %T: %v", err, err)
	}
	if we.Code != code {
		t.Errorf("error code: got %q, want %q (msg: %s)", we.Code, code, we.Message)
	}
}
