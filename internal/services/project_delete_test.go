package services_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// TestDeleteProject_HappyPath deletes an unenrolled, observation-free project
// and verifies its sessions/prompts are removed and it no longer exists.
func TestDeleteProject_HappyPath(t *testing.T) {
	db, _ := newWriteDB(t)

	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('s1', 'repo', '/tmp/repo')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_prompts (session_id, content, project) VALUES ('s1', 'hello', 'repo')`); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}

	res, err := services.DeleteProject(context.Background(), db, "repo")
	if err != nil {
		t.Fatalf("DeleteProject: unexpected error: %v", err)
	}
	if res.Affected.Sessions != 1 {
		t.Errorf("sessions deleted: got %d, want 1", res.Affected.Sessions)
	}
	if res.Affected.UserPrompts != 1 {
		t.Errorf("prompts deleted: got %d, want 1", res.Affected.UserPrompts)
	}

	exists, err := services.ProjectExistsDB(db, "repo")
	if err != nil {
		t.Fatalf("ProjectExistsDB: %v", err)
	}
	if exists {
		t.Error("project 'repo' must not exist after delete")
	}
}

// TestDeleteProject_BlocksEnrolled refuses to delete an enrolled project.
func TestDeleteProject_BlocksEnrolled(t *testing.T) {
	db, _ := newWriteDB(t)

	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('s2', 'synced', '/tmp/synced')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project) VALUES ('synced')`); err != nil {
		t.Fatalf("seed enrollment: %v", err)
	}

	_, err := services.DeleteProject(context.Background(), db, "synced")
	assertWriteErrorCode(t, err, "ENROLLED_UNSUPPORTED")
}

// TestDeleteProject_BlocksWithObservations refuses to delete a project that
// still has live observations (only observation-free projects are deletable).
func TestDeleteProject_BlocksWithObservations(t *testing.T) {
	db, _ := newWriteDB(t)

	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('s3', 'withobs', '/tmp/withobs')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO observations (session_id, type, title, content, project) VALUES ('s3', 'decision', 'T', 'C', 'withobs')`,
	); err != nil {
		t.Fatalf("seed observation: %v", err)
	}

	_, err := services.DeleteProject(context.Background(), db, "withobs")
	assertWriteErrorCode(t, err, "HAS_OBSERVATIONS")
}

// TestDeleteProject_NotFound returns NOT_FOUND for an unknown project.
func TestDeleteProject_NotFound(t *testing.T) {
	db, _ := newWriteDB(t)

	_, err := services.DeleteProject(context.Background(), db, "ghost")
	assertWriteErrorCode(t, err, "NOT_FOUND")
}

// newFKDB builds a minimal DB with the real schema's session_id FOREIGN KEYs and
// foreign_keys enforcement ON. The shared newWriteDB fixture declares no FKs, so
// it cannot catch FK-ordering bugs — these tests must enforce them.
func newFKDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)",
		filepath.Join(dir, "fk.db"),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open fk db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema := []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY, project TEXT NOT NULL, directory TEXT NOT NULL,
			started_at TEXT, ended_at TEXT, summary TEXT
		)`,
		`CREATE TABLE observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
			type TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL,
			project TEXT, scope TEXT NOT NULL DEFAULT 'project',
			updated_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
			content TEXT NOT NULL, project TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE sync_enrolled_projects (project TEXT PRIMARY KEY, enrolled_at TEXT)`,
	}
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// TestDeleteProject_FKEnforced reproduces the real-schema scenario that failed
// in production: a project with a session AND a prompt, with foreign_keys ON.
// Deleting the parent session before its child prompt raises
// "FOREIGN KEY constraint failed"; this pins the child→parent delete order.
func TestDeleteProject_FKEnforced(t *testing.T) {
	db := newFKDB(t)
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('repo-s1','repo','/tmp/repo')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_prompts (session_id, content, project) VALUES ('repo-s1','hi','repo')`); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}

	res, err := services.DeleteProject(context.Background(), db, "repo")
	if err != nil {
		t.Fatalf("DeleteProject under FK enforcement: %v", err)
	}
	if res.Affected.Sessions != 1 || res.Affected.UserPrompts != 1 {
		t.Errorf("affected: %+v, want sessions=1 prompts=1", res.Affected)
	}
	exists, err := services.ProjectExistsDB(db, "repo")
	if err != nil {
		t.Fatalf("ProjectExistsDB: %v", err)
	}
	if exists {
		t.Error("project 'repo' must not exist after delete")
	}
}

// TestDeleteProject_ReassignsForeignObservationSession reproduces the exact case
// the user hit: repo shows 0 observations (its obs were reassigned to "other"),
// but those observations still live in a repo session. Deleting repo must move
// that session to "other" (preserving the observations), delete repo's own
// prompt, and make repo vanish — losing nothing.
func TestDeleteProject_ReassignsForeignObservationSession(t *testing.T) {
	db := newFKDB(t)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed (%s): %v", q, err)
		}
	}
	exec(`INSERT INTO sessions (id, project, directory) VALUES ('repo-s1','repo','/tmp/repo')`)
	// Two observations captured in repo's session but reassigned to "other".
	exec(`INSERT INTO observations (session_id, type, title, content, project) VALUES ('repo-s1','decision','T1','C1','other')`)
	exec(`INSERT INTO observations (session_id, type, title, content, project) VALUES ('repo-s1','decision','T2','C2','other')`)
	// repo's own prompt (should be deleted).
	exec(`INSERT INTO user_prompts (session_id, content, project) VALUES ('repo-s1','p','repo')`)

	res, err := services.DeleteProject(context.Background(), db, "repo")
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if res.Affected.SessionsReassigned != 1 {
		t.Errorf("sessions reassigned: got %d, want 1", res.Affected.SessionsReassigned)
	}
	if res.Affected.Sessions != 0 {
		t.Errorf("sessions deleted: got %d, want 0 (session should be reassigned)", res.Affected.Sessions)
	}
	if res.Affected.UserPrompts != 1 {
		t.Errorf("prompts deleted: got %d, want 1", res.Affected.UserPrompts)
	}

	var sessProject string
	if err := db.QueryRow(`SELECT project FROM sessions WHERE id='repo-s1'`).Scan(&sessProject); err != nil {
		t.Fatalf("session must still exist: %v", err)
	}
	if sessProject != "other" {
		t.Errorf("session project: got %q, want \"other\"", sessProject)
	}
	var obsLeft int
	if err := db.QueryRow(`SELECT COUNT(*) FROM observations WHERE session_id='repo-s1'`).Scan(&obsLeft); err != nil {
		t.Fatalf("count obs: %v", err)
	}
	if obsLeft != 2 {
		t.Errorf("observations preserved: got %d, want 2", obsLeft)
	}

	repoExists, _ := services.ProjectExistsDB(db, "repo")
	if repoExists {
		t.Error("project 'repo' must be gone")
	}
	otherExists, _ := services.ProjectExistsDB(db, "other")
	if !otherExists {
		t.Error("project 'other' must still exist (its observations preserved)")
	}
}
